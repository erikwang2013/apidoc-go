package server

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"slices"
	"time"
	"unicode/utf8"

	"github.com/erikwang2013/apidoc-go/auth"
	"github.com/erikwang2013/apidoc-go/debug"
	"github.com/erikwang2013/apidoc-go/model"
)

const (
	maxDebugBody        = 1 << 20
	maxDebugResponse    = 4 << 20
	maxDebugParams      = 256
	maxDebugParamValue  = 64 << 10
	maxDebugHeaders     = 64
	maxDebugHeaderKey   = 256
	maxDebugHeaderValue = 8 << 10
	maxDebugOptions     = 64
	// File fields are base64-encoded: 256 KiB raw expands to ~342 KiB.
	maxDebugFile        = 256 << 10
	maxDebugOptionValue = 384 << 10
	maxDebugEvents      = 1000
	maxDebugStatus      = 256
	maxDebugEventData   = 64 << 10
)

type debugExecReq struct {
	ID       string            `json:"id"`
	Method   string            `json:"method"`
	URL      string            `json:"url"`
	Params   []debug.Value     `json:"params"`
	Body     []byte            `json:"body"`
	Encoding string            `json:"encoding"`
	Headers  map[string]string `json:"headers"`
	Options  map[string]string `json:"options"`
}

type debugExecResp struct {
	Status     string            `json:"status"`
	Headers    map[string]string `json:"headers,omitempty"`
	Body       []byte            `json:"body,omitempty"`
	Encoding   string            `json:"encoding,omitempty"`
	Events     []debug.Event     `json:"events,omitempty"`
	DurationMS int64             `json:"duration_ms"`
	Truncated  bool              `json:"truncated,omitempty"`
}

// debugExec runs one debugger round trip for a registered action.
func (h *apiHandler) debugExec(w http.ResponseWriter, r *http.Request) {
	select {
	case h.sem <- struct{}{}:
		defer func() { <-h.sem }()
	default:
		errJSON(w, http.StatusTooManyRequests, "too many concurrent debug requests")
		return
	}
	var req debugExecReq
	if !decodeDebugJSON(w, r, &req) || !checkDebugCaps(w, &req) {
		return
	}
	a, ok := h.o.Store.Action(req.ID)
	if !ok {
		errJSON(w, http.StatusNotFound, "action not found")
		return
	}
	if !h.allowDebug(w, r, a.App) {
		return
	}
	d, ok := h.o.Debuggers[a.Protocol]
	if !ok {
		errJSON(w, http.StatusBadRequest, "no debugger for protocol "+a.Protocol)
		return
	}
	spec := d.Spec()
	if spec.Mode == debug.ModeBrowser {
		errJSON(w, http.StatusBadRequest, "protocol is browser-mode")
		return
	}
	if err := checkFileOptions(spec, req.Options); err != nil {
		errJSON(w, http.StatusRequestEntityTooLarge, err.Error())
		return
	}
	method, target, enc, ok := h.resolveDebugTarget(w, a, spec, &req)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), h.debugTimeout())
	defer cancel()
	start := time.Now()
	ev := debug.AuditEvent{
		ActionID: a.ID, App: a.App, Protocol: a.Protocol, Method: method, Target: redactTarget(target),
	}
	if err := h.policyErr(ctx, target); err != nil {
		errJSON(w, http.StatusForbidden, err.Error())
		ev.Status, ev.Duration, ev.Error = auditDenied, time.Since(start), err.Error()
		h.audit(ev)
		return
	}
	resp, err := d.Exec(ctx, debug.Request{
		ActionID: a.ID, Method: method, URL: target, Params: req.Params,
		Body: req.Body, Encoding: enc, Headers: req.Headers, Options: req.Options,
	})
	if err != nil {
		ev.Duration, ev.Error = time.Since(start), err.Error()
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			ev.Status = auditTimeout
			errJSON(w, http.StatusGatewayTimeout, "debug request timed out")
		} else {
			ev.Status = auditError
			errJSON(w, http.StatusBadGateway, err.Error())
		}
		h.audit(ev)
		return
	}
	if resp == nil {
		ev.Status, ev.Duration, ev.Error = auditError, time.Since(start), "debugger returned nil response"
		h.audit(ev)
		errJSON(w, http.StatusBadGateway, "debugger returned nil response")
		return
	}
	status, cut := capUTF8(resp.Status, maxDebugStatus)
	truncated := cut
	body := resp.Body
	if len(body) > maxDebugResponse {
		body = body[:maxDebugResponse]
		truncated = true
	}
	events := make([]debug.Event, 0, min(len(resp.Events), maxDebugEvents))
	for i, ev := range resp.Events {
		if i >= maxDebugEvents {
			truncated = true
			break
		}
		ev, cut = capDebugEvent(ev)
		truncated = truncated || cut
		events = append(events, ev)
	}
	var headers map[string]string
	if len(resp.Headers) > 0 {
		headers = make(map[string]string, min(len(resp.Headers), maxDebugHeaders))
		for k, v := range resp.Headers {
			if len(headers) >= maxDebugHeaders {
				truncated = true
				break
			}
			if len(k) > maxDebugHeaderKey {
				truncated = true
				continue
			}
			hv, cut := capUTF8(v, maxDebugHeaderValue)
			truncated = truncated || cut
			headers[k] = hv
		}
	}
	dur := resp.Duration
	if dur <= 0 {
		dur = time.Since(start)
	}
	ev.Status, ev.Duration, ev.Bytes = resp.Status, dur, len(resp.Body)
	if ev.Status == "" {
		ev.Status = auditOK
	}
	h.audit(ev)
	writeJSON(w, r, http.StatusOK, map[string]any{"code": 0, "msg": "ok", "data": debugExecResp{
		Status: status, Headers: headers, Body: body,
		Encoding: resp.Encoding, Events: events, DurationMS: dur.Milliseconds(),
		Truncated: truncated,
	}})
}

// decodeDebugJSON enforces the JSON content type and the X-Apidoc-Debug
// CSRF header, then decodes at most maxDebugBody. It writes the error
// response itself.
func decodeDebugJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mt != "application/json" {
		errJSON(w, http.StatusUnsupportedMediaType, "content-type must be application/json")
		return false
	}
	if r.Header.Get("X-Apidoc-Debug") != "1" {
		errJSON(w, http.StatusForbidden, "missing X-Apidoc-Debug header")
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxDebugBody)
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			errJSON(w, http.StatusRequestEntityTooLarge, "request body too large")
			return false
		}
		errJSON(w, http.StatusBadRequest, "invalid body")
		return false
	}
	return true
}

// checkFileOptions enforces the raw 256 KiB limit on declared file
// fields. Options carry base64, so the generic option cap is larger.
func checkFileOptions(spec debug.Spec, options map[string]string) error {
	for _, f := range spec.ConnectFields {
		if f.Type != "file" {
			continue
		}
		v, ok := options[f.Name]
		if !ok || v == "" {
			continue
		}
		raw, err := base64.StdEncoding.DecodeString(v)
		if err != nil {
			return fmt.Errorf("file %s is not valid base64", f.Name)
		}
		if len(raw) > maxDebugFile {
			return fmt.Errorf("file %s exceeds 256 KiB", f.Name)
		}
	}
	return nil
}

// checkDebugCaps bounds param, header and option counts and sizes.
func checkDebugCaps(w http.ResponseWriter, req *debugExecReq) bool {
	if len(req.Params) > maxDebugParams {
		errJSON(w, http.StatusBadRequest, "too many params")
		return false
	}
	for _, p := range req.Params {
		if len(p.Value) > maxDebugParamValue {
			errJSON(w, http.StatusBadRequest, "param value too large")
			return false
		}
	}
	if len(req.Headers) > maxDebugHeaders || len(req.Options) > maxDebugOptions {
		errJSON(w, http.StatusBadRequest, "too many headers or options")
		return false
	}
	for k, v := range req.Headers {
		if len(k)+len(v) > maxDebugHeaderValue {
			errJSON(w, http.StatusBadRequest, "header too large")
			return false
		}
	}
	for _, v := range req.Options {
		if len(v) > maxDebugOptionValue {
			errJSON(w, http.StatusBadRequest, "option too large")
			return false
		}
	}
	return true
}

// resolveDebugTarget validates the client-supplied action, target, params
// and encoding against the registered action and its spec. ok=false means
// the error response was already written.
func (h *apiHandler) resolveDebugTarget(w http.ResponseWriter, a *model.Action, spec debug.Spec, req *debugExecReq) (method, target, enc string, ok bool) {
	method = a.Method
	if req.Method != "" && req.Method != a.Method {
		if !slices.Contains(spec.Actions, req.Method) {
			errJSON(w, http.StatusBadRequest, "action not allowed")
			return "", "", "", false
		}
		method = req.Method
	}
	target = a.URL
	if req.URL != "" && req.URL != a.URL {
		if !spec.AllowTargetOverride {
			errJSON(w, http.StatusBadRequest, "target override not allowed")
			return "", "", "", false
		}
		target = req.URL
	}
	allowed := map[string]bool{}
	for _, p := range mergeParams(h.o.GlobalParams, a.Params) {
		allowed[p.Name+"\x00"+p.In] = true
	}
	for _, p := range req.Params {
		if !allowed[p.Name+"\x00"+p.In] {
			errJSON(w, http.StatusBadRequest, "unknown param "+p.Name)
			return "", "", "", false
		}
	}
	enc = req.Encoding
	if enc == "" {
		enc = "json"
	}
	if len(spec.Encodings) > 0 && !slices.Contains(spec.Encodings, enc) {
		errJSON(w, http.StatusBadRequest, "unsupported encoding "+enc)
		return "", "", "", false
	}
	return method, target, enc, true
}

func (h *apiHandler) debugTimeout() time.Duration {
	if h.o.DebugTimeout > 0 {
		return h.o.DebugTimeout
	}
	return 30 * time.Second
}

// capUTF8 truncates s to at most max bytes without splitting a rune.
func capUTF8(s string, max int) (string, bool) {
	if len(s) <= max {
		return s, false
	}
	cut := max
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut], true
}

// capDebugEvent bounds one event's data; cut reports truncation.
func capDebugEvent(ev debug.Event) (debug.Event, bool) {
	d, cut := capUTF8(ev.Data, maxDebugEventData)
	ev.Data = d
	return ev, cut
}

func (h *apiHandler) allowDebug(w http.ResponseWriter, r *http.Request, app string) bool {
	return h.allowDebugToken(w, r, app, "")
}

// allowDebugToken is allowDebug plus an app token that may arrive as a
// query param: an EventSource cannot set headers.
func (h *apiHandler) allowDebugToken(w http.ResponseWriter, r *http.Request, app, queryToken string) bool {
	if h.o.Auth.Enable && !validSession(h.o, r) {
		errJSON(w, http.StatusUnauthorized, "unauthorized")
		return false
	}
	if pw, protected := h.o.AppPWs[app]; protected && pw != "" {
		tok := r.Header.Get("X-Apidoc-App-Token")
		if tok == "" {
			tok = queryToken
		}
		data, ok := auth.Verify(h.o.Auth.Secret, tok)
		if !ok || data != app {
			errJSON(w, http.StatusUnauthorized, "app token required")
			return false
		}
		return true
	}
	if !h.o.Auth.Enable && !h.o.DebugAllowUnauth {
		errJSON(w, http.StatusForbidden, "debugging requires auth or DebugAllowUnauth")
		return false
	}
	return true
}

// specFor returns the registered debugger's panel spec, or the built-in
// browser spec for HTTP. Nil means the protocol has no debugger.
func (h *apiHandler) specFor(protocol string) *debug.Spec {
	if d, ok := h.o.Debuggers[protocol]; ok {
		s := d.Spec()
		return &s
	}
	if protocol == "http" {
		s := debug.HTTPBrowserSpec()
		return &s
	}
	return nil
}

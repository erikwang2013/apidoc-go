package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"

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
	maxDebugHeaderValue = 8 << 10
	maxDebugOptions     = 64
	maxDebugEvents      = 1000
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
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		errJSON(w, http.StatusUnsupportedMediaType, "content-type must be application/json")
		return
	}
	if r.Header.Get("X-Apidoc-Debug") != "1" {
		errJSON(w, http.StatusForbidden, "missing X-Apidoc-Debug header")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxDebugBody)
	var req debugExecReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			errJSON(w, http.StatusRequestEntityTooLarge, "request body too large")
			return
		}
		errJSON(w, http.StatusBadRequest, "invalid body")
		return
	}
	if len(req.Params) > maxDebugParams {
		errJSON(w, http.StatusBadRequest, "too many params")
		return
	}
	for _, p := range req.Params {
		if len(p.Value) > maxDebugParamValue {
			errJSON(w, http.StatusBadRequest, "param value too large")
			return
		}
	}
	if len(req.Headers) > maxDebugHeaders || len(req.Options) > maxDebugOptions {
		errJSON(w, http.StatusBadRequest, "too many headers or options")
		return
	}
	for k, v := range req.Headers {
		if len(k)+len(v) > maxDebugHeaderValue {
			errJSON(w, http.StatusBadRequest, "header too large")
			return
		}
	}
	for k, v := range req.Options {
		if len(k)+len(v) > maxDebugHeaderValue {
			errJSON(w, http.StatusBadRequest, "option too large")
			return
		}
	}
	a, ok := h.o.Store.Action(req.ID)
	if !ok {
		errJSON(w, http.StatusNotFound, "action not found")
		return
	}
	if !h.allowDebug(w, r, a) {
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
	method := a.Method
	if req.Method != "" && req.Method != a.Method {
		if !slices.Contains(spec.Actions, req.Method) {
			errJSON(w, http.StatusBadRequest, "action not allowed")
			return
		}
		method = req.Method
	}
	target := a.URL
	if req.URL != "" && req.URL != a.URL {
		if !spec.AllowTargetOverride {
			errJSON(w, http.StatusBadRequest, "target override not allowed")
			return
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
			return
		}
	}
	enc := req.Encoding
	if enc == "" {
		enc = "json"
	}
	if len(spec.Encodings) > 0 && !slices.Contains(spec.Encodings, enc) {
		errJSON(w, http.StatusBadRequest, "unsupported encoding "+enc)
		return
	}
	timeout := h.o.DebugTimeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()
	start := time.Now()
	resp, err := d.Exec(ctx, debug.Request{
		ActionID: a.ID, Method: method, URL: target, Params: req.Params,
		Body: req.Body, Encoding: enc, Headers: req.Headers, Options: req.Options,
	})
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			errJSON(w, http.StatusGatewayTimeout, "debug request timed out")
			return
		}
		errJSON(w, http.StatusBadGateway, err.Error())
		return
	}
	if resp == nil {
		errJSON(w, http.StatusBadGateway, "debugger returned nil response")
		return
	}
	truncated := false
	body := resp.Body
	if len(body) > maxDebugResponse {
		body = body[:maxDebugResponse]
		truncated = true
	}
	events := resp.Events
	if len(events) > maxDebugEvents {
		events = events[:maxDebugEvents]
		truncated = true
	}
	dur := resp.Duration
	if dur <= 0 {
		dur = time.Since(start)
	}
	writeJSON(w, r, http.StatusOK, map[string]any{"code": 0, "msg": "ok", "data": debugExecResp{
		Status: resp.Status, Headers: resp.Headers, Body: body,
		Encoding: resp.Encoding, Events: events, DurationMS: dur.Milliseconds(),
		Truncated: truncated,
	}})
}

func (h *apiHandler) allowDebug(w http.ResponseWriter, r *http.Request, a *model.Action) bool {
	if h.o.Auth.Enable && !validSession(h.o, r) {
		errJSON(w, http.StatusUnauthorized, "unauthorized")
		return false
	}
	if pw, protected := h.o.AppPWs[a.App]; protected && pw != "" {
		data, ok := auth.Verify(h.o.Auth.Secret, r.Header.Get("X-Apidoc-App-Token"))
		if !ok || data != a.App {
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

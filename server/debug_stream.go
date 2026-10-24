package server

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/erikwang2013/apidoc-go/debug"
)

const (
	maxDebugSessions = 16
	sessionIdle      = 5 * time.Minute
	sessionTTL       = 30 * time.Minute
	maxSessionQueue  = 256
)

// sseHeartbeat is a var so tests can shorten it.
var sseHeartbeat = 15 * time.Second

var (
	errNoSession   = errors.New("session not found")
	errSessionBusy = errors.New("session already has an event stream")
	errTooManySess = errors.New("too many debug sessions")
)

// sessionManager owns open adapter sessions. Expiry is timer-driven: each
// session is reaped after sessionIdle without activity, and never after
// sessionTTL from open.
type sessionManager struct {
	mu       sync.Mutex
	sessions map[string]*streamSession
	max      int
	idle     time.Duration
	ttl      time.Duration
}

type streamSession struct {
	id   string
	app  string
	sess debug.Session
	mgr  *sessionManager

	// Audit metadata for send/close; set before the session is published.
	actionID string
	protocol string
	method   string
	target   string

	events   chan debug.Event
	done     chan struct{}
	once     sync.Once
	started  time.Time
	timer    *time.Timer
	attached bool
}

func newSessionManager(max int, idle, ttl time.Duration) *sessionManager {
	if max <= 0 {
		max = maxDebugSessions
	}
	if idle <= 0 {
		idle = sessionIdle
	}
	if ttl <= 0 {
		ttl = sessionTTL
	}
	return &sessionManager{
		sessions: map[string]*streamSession{}, max: max, idle: idle, ttl: ttl,
	}
}

func (m *sessionManager) open(ctx context.Context, app string, d debug.StreamDebugger, req debug.Request) (*streamSession, error) {
	m.mu.Lock()
	full := len(m.sessions) >= m.max
	m.mu.Unlock()
	if full {
		return nil, errTooManySess
	}
	id, err := m.newID()
	if err != nil {
		return nil, err
	}
	// Open outside the lock: adapters dial the network here.
	ds, err := d.Open(ctx, req)
	if err != nil {
		return nil, err
	}
	if ds == nil {
		return nil, errors.New("stream debugger returned nil session")
	}
	s := &streamSession{
		id: id, app: app, sess: ds, mgr: m,
		actionID: req.ActionID, protocol: d.Protocol(),
		method: req.Method, target: req.URL,
		events:  make(chan debug.Event, maxSessionQueue),
		done:    make(chan struct{}),
		started: time.Now(),
	}
	s.timer = time.AfterFunc(m.idleLeft(s.started), func() { m.remove(id) })
	m.mu.Lock()
	if len(m.sessions) >= m.max {
		m.mu.Unlock()
		s.close()
		return nil, errTooManySess
	}
	m.sessions[id] = s
	m.mu.Unlock()
	go s.pump()
	return s, nil
}

// newID returns 256 random bits, URL-safe without padding.
func (m *sessionManager) newID() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// idleLeft caps the idle timeout at the session's remaining total lifetime.
func (m *sessionManager) idleLeft(started time.Time) time.Duration {
	left := time.Until(started.Add(m.ttl))
	if left <= 0 {
		return time.Nanosecond
	}
	if left < m.idle {
		return left
	}
	return m.idle
}

func (m *sessionManager) get(id string) (*streamSession, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[id]
	return s, ok
}

func (m *sessionManager) attach(id string) (*streamSession, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[id]
	if !ok {
		return nil, errNoSession
	}
	if s.attached {
		return nil, errSessionBusy
	}
	s.attached = true
	return s, nil
}

func (m *sessionManager) remove(id string) {
	m.mu.Lock()
	s := m.sessions[id]
	delete(m.sessions, id)
	m.mu.Unlock()
	if s != nil {
		s.close()
	}
}

// touch restarts the idle timer after Send or event activity.
func (s *streamSession) touch() {
	s.mgr.mu.Lock()
	defer s.mgr.mu.Unlock()
	if _, ok := s.mgr.sessions[s.id]; !ok {
		return
	}
	s.timer.Reset(s.mgr.idleLeft(s.started))
}

func (s *streamSession) close() {
	s.once.Do(func() {
		s.timer.Stop()
		close(s.done)
		_ = s.sess.Close()
	})
}

// auditEvent carries the session's immutable identity for send/close events.
func (s *streamSession) auditEvent() debug.AuditEvent {
	return debug.AuditEvent{
		ActionID: s.actionID, App: s.app, Protocol: s.protocol,
		Method: s.method, Target: s.target,
	}
}

// pump forwards adapter events into the bounded queue. When the queue is
// full it blocks, which backpressures the adapter instead of growing memory.
func (s *streamSession) pump() {
	defer close(s.events)
	for {
		select {
		case ev, ok := <-s.sess.Events():
			if !ok {
				return
			}
			select {
			case s.events <- ev:
				s.touch()
			case <-s.done:
				return
			}
		case <-s.done:
			return
		}
	}
}

type debugSendReq struct {
	SessionID string            `json:"session_id"`
	Body      []byte            `json:"body"`
	Encoding  string            `json:"encoding"`
	Meta      map[string]string `json:"meta"`
}

type debugCloseReq struct {
	SessionID string `json:"session_id"`
}

// debugOpen starts one streaming session for a registered action.
func (h *apiHandler) debugOpen(w http.ResponseWriter, r *http.Request) {
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
	sd, ok := d.(debug.StreamDebugger)
	if !ok {
		errJSON(w, http.StatusBadRequest, "protocol does not support streaming")
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
		ActionID: a.ID, App: a.App, Protocol: a.Protocol, Method: method, Target: target,
	}
	if err := h.policyErr(ctx, target); err != nil {
		errJSON(w, http.StatusForbidden, err.Error())
		ev.Status, ev.Duration, ev.Error = auditDenied, time.Since(start), err.Error()
		h.audit(ev)
		return
	}
	s, err := h.sessions.open(ctx, a.App, sd, debug.Request{
		ActionID: a.ID, Method: method, URL: target, Params: req.Params,
		Body: req.Body, Encoding: enc, Headers: req.Headers, Options: req.Options,
	})
	if err != nil {
		ev.Duration, ev.Error = time.Since(start), err.Error()
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			ev.Status = auditTimeout
			errJSON(w, http.StatusGatewayTimeout, "debug open timed out")
		} else if errors.Is(err, errTooManySess) {
			ev.Status = auditError
			errJSON(w, http.StatusTooManyRequests, err.Error())
		} else {
			ev.Status = auditError
			errJSON(w, http.StatusBadGateway, err.Error())
		}
		h.audit(ev)
		return
	}
	ev.Status, ev.Duration = auditOK, time.Since(start)
	h.audit(ev)
	writeJSON(w, r, http.StatusOK, map[string]any{"code": 0, "msg": "ok",
		"data": map[string]any{"session_id": s.id}})
}

// debugSend writes one message to an open session.
func (h *apiHandler) debugSend(w http.ResponseWriter, r *http.Request) {
	var req debugSendReq
	if !decodeDebugJSON(w, r, &req) {
		return
	}
	if len(req.Body) > maxDebugBody {
		errJSON(w, http.StatusRequestEntityTooLarge, "message too large")
		return
	}
	if len(req.Meta) > maxDebugOptions {
		errJSON(w, http.StatusBadRequest, "too many meta fields")
		return
	}
	for k, v := range req.Meta {
		if len(k)+len(v) > maxDebugHeaderValue {
			errJSON(w, http.StatusBadRequest, "meta field too large")
			return
		}
	}
	s, ok := h.sessions.get(req.SessionID)
	if !ok {
		errJSON(w, http.StatusNotFound, "session not found")
		return
	}
	if !h.allowDebug(w, r, s.app) {
		return
	}
	if req.Encoding != "" {
		if req.Meta == nil {
			req.Meta = map[string]string{}
		}
		if _, ok := req.Meta["encoding"]; !ok {
			req.Meta["encoding"] = req.Encoding
		}
	}
	s.touch()
	ctx, cancel := context.WithTimeout(r.Context(), h.debugTimeout())
	defer cancel()
	start := time.Now()
	ev := s.auditEvent()
	ev.Bytes = len(req.Body)
	if err := s.sess.Send(ctx, req.Body, req.Meta); err != nil {
		ev.Duration, ev.Error = time.Since(start), err.Error()
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			ev.Status = auditTimeout
			errJSON(w, http.StatusGatewayTimeout, "debug send timed out")
		} else {
			ev.Status = auditError
			errJSON(w, http.StatusBadGateway, err.Error())
		}
		h.audit(ev)
		return
	}
	ev.Status, ev.Duration = auditOK, time.Since(start)
	h.audit(ev)
	writeJSON(w, r, http.StatusOK, map[string]any{"code": 0, "msg": "ok"})
}

// debugClose ends one session; the SSE reader (if any) sees the queue close.
func (h *apiHandler) debugClose(w http.ResponseWriter, r *http.Request) {
	var req debugCloseReq
	if !decodeDebugJSON(w, r, &req) {
		return
	}
	s, ok := h.sessions.get(req.SessionID)
	if !ok {
		errJSON(w, http.StatusNotFound, "session not found")
		return
	}
	if !h.allowDebug(w, r, s.app) {
		return
	}
	start := time.Now()
	h.sessions.remove(req.SessionID)
	ev := s.auditEvent()
	ev.Status, ev.Duration = auditOK, time.Since(start)
	h.audit(ev)
	writeJSON(w, r, http.StatusOK, map[string]any{"code": 0, "msg": "ok"})
}

// debugEvents streams session events as SSE. EventSource cannot set
// headers, so the app token may arrive as app_token; the 256-bit session
// ID is the CSRF token here. A client disconnect closes the session.
func (h *apiHandler) debugEvents(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("session_id")
	if id == "" {
		errJSON(w, http.StatusBadRequest, "missing session_id")
		return
	}
	s, ok := h.sessions.get(id)
	if !ok {
		errJSON(w, http.StatusNotFound, "session not found")
		return
	}
	if !h.allowDebugToken(w, r, s.app, r.URL.Query().Get("app_token")) {
		return
	}
	s, err := h.sessions.attach(id)
	if err != nil {
		if errors.Is(err, errSessionBusy) {
			errJSON(w, http.StatusConflict, err.Error())
			return
		}
		errJSON(w, http.StatusNotFound, err.Error())
		return
	}
	defer h.sessions.remove(id)
	fl, ok := w.(http.Flusher)
	if !ok {
		errJSON(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	fl.Flush()

	ticker := time.NewTicker(sseHeartbeat)
	defer ticker.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			if _, err := io.WriteString(w, ": ping\n\n"); err != nil {
				return
			}
			fl.Flush()
		case ev, ok := <-s.events:
			if !ok {
				_, _ = io.WriteString(w, "event: end\ndata: {}\n\n")
				fl.Flush()
				return
			}
			var cut bool
			ev, cut = capDebugEvent(ev)
			if cut {
				ev.Name = strings.TrimSpace(ev.Name + " (truncated)")
			}
			b, err := json.Marshal(ev)
			if err != nil {
				continue
			}
			if _, err := io.WriteString(w, "data: "+string(b)+"\n\n"); err != nil {
				return
			}
			fl.Flush()
		}
	}
}

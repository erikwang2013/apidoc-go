package uiapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/erikwang2013/apidoc-go/auth"
	"github.com/erikwang2013/apidoc-go/model"
	debug "github.com/erikwang2013/apidoc-go/server/uiapi/debug"
)

const (
	debugMaxSessions   = 4
	debugSessionTTL    = 60 * time.Second
	debugFrameCap      = 1000
	debugPollDrain     = 100
	debugEnvelopeLimit = 8 << 20
)

type debugInput struct {
	Headers  map[string]string `json:"headers"`
	Query    map[string]string `json:"query"`
	Fields   map[string]any    `json:"fields"`
	Payload  string            `json:"payload"`
	Encoding string            `json:"encoding"`
	Timeout  int               `json:"timeout"`
}

type debugSendBody struct {
	AppKey   string     `json:"appKey"`
	ShareKey string     `json:"shareKey"`
	Path     string     `json:"path"`
	Input    debugInput `json:"input"`
}

type debugSessionBody struct {
	SessionID string `json:"sessionId"`
	AppKey    string `json:"appKey"`
	ShareKey  string `json:"shareKey"`
}

// decodeDebug allows the 1 MiB payload plus JSON escaping overhead.
func decodeDebug[T any](r *http.Request) (T, error) {
	var value T
	err := json.NewDecoder(io.LimitReader(r.Body, debugEnvelopeLimit)).Decode(&value)
	return value, err
}

func (h *handler) debugSend(w http.ResponseWriter, r *http.Request) {
	body, err := decodeDebug[debugSendBody](r)
	if err != nil {
		writeError(w, http.StatusBadRequest, 400, "invalid body")
		return
	}
	action, allowed := h.debugAction(w, r, body)
	if !allowed {
		return
	}
	if len(body.Input.Payload) > debug.MaxMessageBytes {
		writeError(w, http.StatusRequestEntityTooLarge, 413, "message too large")
		return
	}
	if !h.o.Debug.EnableServerSide {
		writeJSON(w, http.StatusOK, ok(okDebugFailure("server-side debug is disabled")))
		return
	}
	resp, err := debug.Execute(h.o.Debug, h.debugRequest(action, body.Input))
	if err != nil {
		writeJSON(w, http.StatusOK, ok(okDebugFailure(err.Error())))
		return
	}
	data := map[string]any{
		"ok":       true,
		"status":   resp.Status,
		"headers":  resp.Headers,
		"body":     resp.Body.Data,
		"encoding": debugEncoding(resp.Body.Encoding),
		"meta":     map[string]any{"ms": resp.MS},
	}
	writeJSON(w, http.StatusOK, ok(data))
}

func (h *handler) debugStart(w http.ResponseWriter, r *http.Request) {
	body, err := decodeDebug[debugSendBody](r)
	if err != nil {
		writeError(w, http.StatusBadRequest, 400, "invalid body")
		return
	}
	action, allowed := h.debugAction(w, r, body)
	if !allowed {
		return
	}
	if len(body.Input.Payload) > debug.MaxMessageBytes {
		writeError(w, http.StatusRequestEntityTooLarge, 413, "message too large")
		return
	}
	if !h.o.Debug.EnableServerSide {
		writeJSON(w, http.StatusOK, ok(okDebugFailure("server-side debug is disabled")))
		return
	}
	appKey := canonicalAppKey(action.App, action.Version)
	id, reserved := h.debugSessions.reserve(h.debugScopeKey(r, appKey, body.ShareKey), appKey, body.ShareKey)
	if !reserved {
		writeJSON(w, http.StatusOK, ok(okDebugFailure("too many active sessions")))
		return
	}
	stream, err := debug.OpenStream(h.o.Debug, h.debugRequest(action, body.Input))
	if err != nil {
		h.debugSessions.remove(id)
		writeJSON(w, http.StatusOK, ok(okDebugFailure(err.Error())))
		return
	}
	if !h.debugSessions.attach(id, stream.Close) {
		_ = stream.Close()
		writeJSON(w, http.StatusOK, ok(okDebugFailure("session expired")))
		return
	}
	go h.collectDebugStream(id, stream)
	writeJSON(w, http.StatusOK, ok(map[string]any{"ok": true, "sessionId": id}))
}

func (h *handler) debugPoll(w http.ResponseWriter, r *http.Request) {
	body, err := decode[debugSessionBody](r)
	if err != nil {
		writeError(w, http.StatusBadRequest, 400, "invalid body")
		return
	}
	appKey, shareKey, alive := h.debugSessions.bindings(body.SessionID)
	if !alive {
		appKey, shareKey = body.AppKey, body.ShareKey
	}
	if !h.debugSessionAuth(w, r, appKey, shareKey, body.ShareKey) {
		return
	}
	if !alive {
		writeJSON(w, http.StatusOK, ok(okDebugFailure("session not found or expired")))
		return
	}
	messages, closeUp, reached, found := h.debugSessions.poll(body.SessionID)
	if !found {
		writeJSON(w, http.StatusOK, ok(okDebugFailure("session not found or expired")))
		return
	}
	if closeUp != nil {
		_ = closeUp()
	}
	if reached {
		messages = append(messages, map[string]any{"type": "error", "body": "max_frames reached"})
	}
	if messages == nil {
		messages = []map[string]any{}
	}
	writeJSON(w, http.StatusOK, ok(map[string]any{"ok": true, "messages": messages}))
}

func (h *handler) debugStop(w http.ResponseWriter, r *http.Request) {
	body, err := decode[debugSessionBody](r)
	if err != nil {
		writeError(w, http.StatusBadRequest, 400, "invalid body")
		return
	}
	appKey, shareKey, alive := h.debugSessions.bindings(body.SessionID)
	if !alive {
		appKey, shareKey = body.AppKey, body.ShareKey
	}
	if !h.debugSessionAuth(w, r, appKey, shareKey, body.ShareKey) {
		return
	}
	if closeUp := h.debugSessions.remove(body.SessionID); closeUp != nil {
		_ = closeUp()
	}
	writeJSON(w, http.StatusOK, ok(map[string]any{"ok": true}))
}

// debugAction resolves the endpoint, enforces the client appKey match, and
// applies the normal share/app/global credential rules. A share that does not
// cover the endpoint is indistinguishable from a missing action.
func (h *handler) debugAction(w http.ResponseWriter, r *http.Request, body debugSendBody) (*model.Action, bool) {
	action, found := h.o.Store.Action(body.Path)
	if !found {
		writeError(w, http.StatusNotFound, 404, "action not found")
		return nil, false
	}
	appKey := canonicalAppKey(action.App, action.Version)
	if body.AppKey != "" && body.AppKey != appKey {
		writeError(w, http.StatusNotFound, 404, "action not found")
		return nil, false
	}
	rec, allowed := h.authorize(w, r, appKey, body.ShareKey)
	if !allowed {
		return nil, false
	}
	if !rec.apiAllowed(action.ID) {
		writeError(w, http.StatusNotFound, 404, "action not found")
		return nil, false
	}
	return action, true
}

// debugRequest builds the executor request. Target and reserved opts come only
// from server-side annotations; client input can never change the host/port.
func (h *handler) debugRequest(action *model.Action, input debugInput) *debug.Request {
	kind := strings.ToLower(strings.TrimSpace(action.Protocol))
	if kind == "" {
		kind = "http"
	}
	if kind == "rawsocket" {
		kind = "raw"
	}
	opts := map[string]string{}
	for key, value := range action.ProtocolOpts {
		if key != "target" {
			opts[key] = value
		}
	}
	for key, value := range input.Fields {
		if key == "target" {
			continue
		}
		opts[key] = debugFieldValue(value)
	}
	target := action.ProtocolOpts["target"]
	if target == "" {
		target = action.URL
	}
	var payload *debug.Payload
	if input.Payload != "" {
		encoding := input.Encoding
		if encoding == "" {
			encoding = "utf8"
		}
		payload = &debug.Payload{Encoding: encoding, Data: input.Payload}
	}
	timeoutMS := 0
	if input.Timeout > 0 {
		timeoutMS = input.Timeout * 1000
	}
	return &debug.Request{
		Kind:      kind,
		Target:    target,
		Method:    action.Method,
		Headers:   input.Headers,
		Body:      payload,
		Opts:      opts,
		Endpoint:  debugEndpoint(action),
		TimeoutMS: timeoutMS,
	}
}

func debugEndpoint(action *model.Action) *debug.Endpoint {
	endpoint := &debug.Endpoint{
		URL:      action.URL,
		Method:   action.Method,
		Protocol: action.Protocol,
		Params:   debugParams(action.Params),
	}
	for _, response := range action.Responses {
		endpoint.Returned = append(endpoint.Returned, debugParams(response.Fields)...)
	}
	return endpoint
}

func debugParams(params []model.Param) []debug.Param {
	if len(params) == 0 {
		return nil
	}
	out := make([]debug.Param, 0, len(params))
	for _, p := range params {
		out = append(out, debug.Param{
			Name: p.Name, Type: p.Type, Field: p.Field, Required: p.Required,
			Children: debugParams(p.Children),
		})
	}
	return out
}

// debugFieldValue mirrors Rust's Value::to_string for non-string fields.
func debugFieldValue(value any) string {
	if text, ok := value.(string); ok {
		return text
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return string(encoded)
}

// debugScopeKey is the session quota key: one pool per share or token scope.
func (h *handler) debugScopeKey(r *http.Request, appKey, shareKey string) string {
	if shareKey != "" {
		return "share:" + shareKey
	}
	if token := bearerToken(r); token != "" {
		if scope, found := auth.Verify(h.o.Auth.Secret, token); found {
			return "token:" + scope
		}
	}
	return "app:" + appKey
}

// debugSessionAuth checks a poll/stop against the scope recorded at start.
// Share sessions additionally require the same shareKey from the client.
func (h *handler) debugSessionAuth(w http.ResponseWriter, r *http.Request, appKey, sessionShareKey, clientShareKey string) bool {
	if sessionShareKey == "" {
		if !h.canAccess(r, appKey) {
			writeError(w, http.StatusUnauthorized, 401, "token error")
			return false
		}
		return true
	}
	if clientShareKey != sessionShareKey {
		writeError(w, http.StatusUnauthorized, 401, "token error")
		return false
	}
	_, ok := h.shareAuth(w, r, sessionShareKey)
	return ok
}

func (h *handler) collectDebugStream(id string, stream debug.StreamSession) {
	defer stream.Close()
	for {
		frame, err := stream.ReadFrame()
		if err != nil {
			message := map[string]any{"type": "close", "body": "upstream closed"}
			if !errors.Is(err, io.EOF) {
				message = map[string]any{"type": "error", "body": err.Error()}
			}
			h.debugSessions.appendMessage(id, message)
			return
		}
		payload := debug.FromBytes(frame)
		message := map[string]any{
			"time":     strconv.FormatInt(time.Now().UnixMilli(), 10),
			"type":     "frame",
			"body":     payload.Data,
			"encoding": debugEncoding(payload.Encoding),
		}
		if !h.debugSessions.appendMessage(id, message) {
			return
		}
	}
}

func debugEncoding(encoding string) string {
	switch encoding {
	case "base64", "hex":
		return encoding
	default:
		return "text"
	}
}

func okDebugFailure(message string) map[string]any {
	return map[string]any{"ok": false, "error": message}
}

// debugSessionStore keeps stream sessions in memory, one connection budget per
// credential scope. The timer is the idle TTL; a poll refreshes it.
type debugSessionStore struct {
	mu       sync.Mutex
	ttl      time.Duration
	sessions map[string]*debugSession
	perScope map[string]int
}

type debugSession struct {
	scope     string
	appKey    string
	shareKey  string
	messages  []map[string]any
	delivered int
	timer     *time.Timer
	closeUp   func() error
}

func newDebugSessionStore(ttl time.Duration) *debugSessionStore {
	return &debugSessionStore{
		ttl:      ttl,
		sessions: map[string]*debugSession{},
		perScope: map[string]int{},
	}
}

func (s *debugSessionStore) reserve(scope, appKey, shareKey string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.perScope[scope] >= debugMaxSessions {
		return "", false
	}
	var id string
	for attempt := 0; attempt < 5; attempt++ {
		candidate, err := randomHex(16)
		if err != nil {
			return "", false
		}
		if _, exists := s.sessions[candidate]; !exists {
			id = candidate
			break
		}
	}
	if id == "" {
		return "", false
	}
	session := &debugSession{scope: scope, appKey: appKey, shareKey: shareKey}
	session.timer = time.AfterFunc(s.ttl, func() {
		if closeUp := s.remove(id); closeUp != nil {
			_ = closeUp()
		}
	})
	s.sessions[id] = session
	s.perScope[scope]++
	return id, true
}

func (s *debugSessionStore) attach(id string, closeUp func() error) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, found := s.sessions[id]
	if !found {
		return false
	}
	session.closeUp = closeUp
	return true
}

func (s *debugSessionStore) bindings(id string) (appKey, shareKey string, ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, found := s.sessions[id]
	if !found {
		return "", "", false
	}
	return session.appKey, session.shareKey, true
}

func (s *debugSessionStore) appendMessage(id string, message map[string]any) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, found := s.sessions[id]
	if !found {
		return false
	}
	if len(session.messages) >= debugFrameCap {
		return false
	}
	session.messages = append(session.messages, message)
	return true
}

// poll drains up to debugPollDrain messages and returns the upstream closer
// when the delivered-frame budget is exhausted.
func (s *debugSessionStore) poll(id string) ([]map[string]any, func() error, bool, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, found := s.sessions[id]
	if !found {
		return nil, nil, false, false
	}
	if session.timer != nil {
		session.timer.Reset(s.ttl)
	}
	budget := debugFrameCap - session.delivered
	count := len(session.messages)
	if count > debugPollDrain {
		count = debugPollDrain
	}
	if count > budget {
		count = budget
	}
	messages := append([]map[string]any{}, session.messages[:count]...)
	session.messages = session.messages[count:]
	session.delivered += count
	if session.delivered < debugFrameCap {
		return messages, nil, false, true
	}
	delete(s.sessions, id)
	if session.timer != nil {
		session.timer.Stop()
	}
	s.perScope[sessionScope(session)]--
	if s.perScope[sessionScope(session)] <= 0 {
		delete(s.perScope, sessionScope(session))
	}
	return messages, session.closeUp, true, true
}

func (s *debugSessionStore) remove(id string) func() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, found := s.sessions[id]
	if !found {
		return nil
	}
	delete(s.sessions, id)
	if session.timer != nil {
		session.timer.Stop()
	}
	s.perScope[sessionScope(session)]--
	if s.perScope[sessionScope(session)] <= 0 {
		delete(s.perScope, sessionScope(session))
	}
	return session.closeUp
}

// count is the test-visible session count for one quota scope.
func (s *debugSessionStore) count(scope string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.perScope[scope]
}

// sessionScope is tracked on the session so removal does not need the caller's
// scope string; reserve stores it on the session for this purpose.
func sessionScope(session *debugSession) string { return session.scope }

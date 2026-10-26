package uiapi

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/erikwang2013/apidoc-go/auth"
	"github.com/erikwang2013/apidoc-go/model"
	debug "github.com/erikwang2013/apidoc-go/server/uiapi/debug"
	"github.com/erikwang2013/apidoc-go/store"
)

func debugActionFixture() *model.Action {
	return &model.Action{
		App: "default", Version: "v1", Method: "SUBSCRIBE",
		URL:      "ws://example.com/chat",
		Protocol: "ws",
		ProtocolOpts: map[string]string{
			"target": "wss://configured.example/ws",
			"room":   "lobby",
		},
		Params: []model.Param{
			{Name: "user", Type: "string", Field: "1"},
		},
		Responses: []model.Response{
			{Name: "ok", Fields: []model.Param{{Name: "id", Type: "i32", Field: "1"}}},
		},
	}
}

func TestDebugSendDisabledFailsClosed(t *testing.T) {
	h := newHandler(t)
	token := login(t, h)
	itemsID := store.ActionID("default", "v1", "GET", "/items")
	rec := do(t, h, "POST", "/ui/v1/debugSend",
		`{"appKey":"default|v1","path":"`+itemsID+`","input":{"payload":"","encoding":"text","timeout":5}}`, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("debugSend: %d %s", rec.Code, rec.Body.String())
	}
	data := decodeData(t, rec)
	if data["ok"] != false || data["error"] != "server-side debug is disabled" {
		t.Fatalf("disabled debug must fail closed: %+v", data)
	}

	rec = do(t, h, "POST", "/ui/v1/debugStart",
		`{"appKey":"default|v1","path":"`+itemsID+`","input":{}}`, token)
	data = decodeData(t, rec)
	if data["ok"] != false || data["sessionId"] != nil {
		t.Fatalf("debugStart must not create a session when disabled: %+v", data)
	}
}

func TestDebugSendScope(t *testing.T) {
	h := newHandlerWith(t, Options{
		Store:  newMultiAppStore(),
		Auth:   auth.Config{Enable: true, Password: "pw", Secret: "secret", Expire: time.Hour},
		AppPWs: map[string]string{"b": "bpw"},
	})
	itemsID := store.ActionID("default", "v1", "GET", "/items")
	body := `{"appKey":"default|v1","path":"` + itemsID + `","input":{}}`

	// An app token for B cannot execute an endpoint owned by default.
	bToken := loginApp(t, h, "b|v1", "bpw")
	if rec := do(t, h, "POST", "/ui/v1/debugSend", body, bToken); rec.Code != http.StatusUnauthorized {
		t.Fatalf("cross-app token: %d %s", rec.Code, rec.Body.String())
	}
	// A client-supplied appKey naming another app is a 404, never a redirect.
	session := login(t, h)
	mismatch := `{"appKey":"b|v1","path":"` + itemsID + `","input":{}}`
	if rec := do(t, h, "POST", "/ui/v1/debugSend", mismatch, session); rec.Code != http.StatusNotFound {
		t.Fatalf("appKey mismatch: %d %s", rec.Code, rec.Body.String())
	}
}

func TestDebugShareScope(t *testing.T) {
	h := newHandlerWith(t, Options{
		Store: newMultiAppStore(),
		Auth:  auth.Config{Enable: true, Password: "pw", Secret: "secret", Expire: time.Hour},
	})
	admin := login(t, h)
	itemsID := store.ActionID("default", "v1", "GET", "/items")
	wsID := store.ActionID("default", "v1", "SUBSCRIBE", "ws://example.com/chat")
	rec := do(t, h, "POST", "/ui/v1/addApiShare",
		`{"key":"s2","name":"One API","type":"api","appKeys":["default"],"apiKeys":["`+itemsID+`"],"password":"sp"}`, admin)
	if !decodeDataOf[bool](t, rec) {
		t.Fatalf("add share: %s", rec.Body.String())
	}
	token := decodeData(t, do(t, h, "POST", "/ui/v1/verifyAuth", `{"shareKey":"s2","password":"sp"}`, ""))["token"].(string)

	inside := `{"appKey":"default|v1","path":"` + itemsID + `","shareKey":"s2","input":{}}`
	data := decodeData(t, do(t, h, "POST", "/ui/v1/debugSend", inside, token))
	if data["ok"] != false || data["error"] != "server-side debug is disabled" {
		t.Fatalf("share inside scope should reach the disabled gate: %+v", data)
	}
	outside := `{"appKey":"default|v1","path":"` + wsID + `","shareKey":"s2","input":{}}`
	if rec := do(t, h, "POST", "/ui/v1/debugSend", outside, token); rec.Code != http.StatusNotFound {
		t.Fatalf("share outside scope: %d %s", rec.Code, rec.Body.String())
	}
}

func TestDebugPayloadCap(t *testing.T) {
	h := newHandler(t)
	token := login(t, h)
	itemsID := store.ActionID("default", "v1", "GET", "/items")
	payload := strings.Repeat("x", debug.MaxMessageBytes+1)
	body := `{"appKey":"default|v1","path":"` + itemsID + `","input":{"payload":"` + payload + `"}}`
	rec := do(t, h, "POST", "/ui/v1/debugSend", body, token)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversize payload: %d %s", rec.Code, rec.Body.String())
	}
}

func TestDebugRequestTargetOnlyFromServerConfig(t *testing.T) {
	h := &handler{o: Options{}}
	action := debugActionFixture()
	req := h.debugRequest(action, debugInput{
		Fields: map[string]any{"target": "http://169.254.169.254/", "room": "b"},
	})
	if req.Target != "wss://configured.example/ws" {
		t.Fatalf("client target must be ignored: %q", req.Target)
	}
	if req.Opts["room"] != "b" || req.Opts["target"] != "" {
		t.Fatalf("reserved opts leaked into executors: %+v", req.Opts)
	}
}

func TestDebugSessionStoreQuotaAndTTL(t *testing.T) {
	s := newDebugSessionStore(20 * time.Millisecond)
	for i := 0; i < debugMaxSessions; i++ {
		if _, ok := s.reserve("scope-a", "default|v1", ""); !ok {
			t.Fatalf("reserve %d failed", i)
		}
	}
	if _, ok := s.reserve("scope-a", "default|v1", ""); ok {
		t.Fatal("fifth session in one scope must be rejected")
	}
	if _, ok := s.reserve("scope-b", "default|v1", ""); !ok {
		t.Fatal("other scope must get its own session budget")
	}
	time.Sleep(60 * time.Millisecond)
	if got := s.count("scope-a"); got != 0 {
		t.Fatalf("idle sessions not reaped: %d", got)
	}
}

func TestDebugSessionStoreCapsAndDrain(t *testing.T) {
	s := newDebugSessionStore(time.Minute)
	id, ok := s.reserve("scope", "default|v1", "")
	if !ok {
		t.Fatal("reserve failed")
	}
	closed := 0
	if !s.attach(id, func() error { closed++; return nil }) {
		t.Fatal("attach failed")
	}
	for i := 0; i < debugFrameCap; i++ {
		if !s.appendMessage(id, map[string]any{"type": "frame"}) {
			t.Fatalf("append %d failed before cap", i)
		}
	}
	if s.appendMessage(id, map[string]any{"type": "frame"}) {
		t.Fatal("append past the frame cap must fail")
	}
	reached := false
	for i := 0; i < debugFrameCap/debugPollDrain; i++ {
		messages, closeUp, capped, found := s.poll(id)
		if !found {
			t.Fatalf("session vanished in poll %d", i)
		}
		if len(messages) != debugPollDrain {
			t.Fatalf("poll %d drained %d messages", i, len(messages))
		}
		if capped {
			reached = true
			if closeUp != nil {
				_ = closeUp()
			}
		}
	}
	if !reached || closed != 1 {
		t.Fatalf("frame cap not enforced: reached=%v closed=%d", reached, closed)
	}
	if _, _, _, found := s.poll(id); found {
		t.Fatal("session must be removed at the frame cap")
	}
}

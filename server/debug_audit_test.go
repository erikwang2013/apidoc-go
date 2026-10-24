package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/erikwang2013/apidoc-go/debug"
	"github.com/erikwang2013/apidoc-go/debug/debugtest"
	"github.com/erikwang2013/apidoc-go/store"
)

func TestDebugExecAudit(t *testing.T) {
	d := &debugtest.FakeAdapter{
		Proto: "mqtt",
		SpecValue: debug.Spec{
			Mode: debug.ModeServer, Actions: []string{"PUBLISH"},
		},
		Response: &debug.Response{Status: "PUBACK", Body: []byte("resp")},
	}
	var events []debug.AuditEvent
	h := Handler(Opts{Prefix: "/apidoc", Store: newDebugStore(),
		Debuggers: map[string]debug.Debugger{"mqtt": d}, DebugAllowUnauth: true,
		DebugAudit: func(e debug.AuditEvent) { events = append(events, e) }})
	id := store.ActionID("a", "v1", "PUBLISH", "mqtt://broker/topic")
	body := fmt.Sprintf(`{"id":%q,"method":"PUBLISH","url":"mqtt://broker/topic",`+
		`"params":[{"name":"payload","in":"payload","value":"secret-param"}],`+
		`"body":"c2VjcmV0LWJvZHk=","encoding":"json",`+
		`"options":{"clientId":"secret-option"}}`, id)
	if rec := debugPost(t, h, body, true); rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if len(events) != 1 {
		t.Fatalf("audit events = %d", len(events))
	}
	ev := events[0]
	if ev.Time.IsZero() || ev.ActionID != id || ev.App != "a" || ev.Protocol != "mqtt" ||
		ev.Method != "PUBLISH" || ev.Target != "mqtt://broker/topic" ||
		ev.Status != "PUBACK" || ev.Bytes != len("resp") || ev.Error != "" ||
		ev.Duration < 0 {
		t.Fatalf("audit event wrong: %+v", ev)
	}
	// The event must not carry params, body or options values.
	b, err := json.Marshal(ev)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"secret-param", "secret-body", "secret-option"} {
		if strings.Contains(string(b), secret) {
			t.Fatalf("audit leaked %q: %s", secret, b)
		}
	}
}

func TestDebugAuditErrorAndPanic(t *testing.T) {
	spec := debug.Spec{Mode: debug.ModeServer, Actions: []string{"PUBLISH"}}
	id := store.ActionID("a", "v1", "PUBLISH", "mqtt://broker/topic")
	body := fmt.Sprintf(`{"id":%q,"method":"PUBLISH","url":"mqtt://broker/topic",`+
		`"params":[],"body":"","encoding":"json"}`, id)

	var events []debug.AuditEvent
	h := Handler(Opts{Prefix: "/apidoc", Store: newDebugStore(),
		Debuggers: map[string]debug.Debugger{"mqtt": &debugtest.FakeAdapter{
			Proto: "mqtt", SpecValue: spec, Err: errors.New("boom"),
		}}, DebugAllowUnauth: true,
		DebugAudit: func(e debug.AuditEvent) { events = append(events, e) }})
	if rec := debugPost(t, h, body, true); rec.Code != http.StatusBadGateway {
		t.Fatalf("error status %d", rec.Code)
	}
	if len(events) != 1 || events[0].Status != auditError || events[0].Error != "boom" {
		t.Fatalf("error audit = %+v", events)
	}

	// A panicking audit callback must not break the response.
	h = Handler(Opts{Prefix: "/apidoc", Store: newDebugStore(),
		Debuggers: map[string]debug.Debugger{"mqtt": &debugtest.FakeAdapter{
			Proto: "mqtt", SpecValue: spec,
		}}, DebugAllowUnauth: true,
		DebugAudit: func(debug.AuditEvent) { panic("audit boom") }})
	if rec := debugPost(t, h, body, true); rec.Code != http.StatusOK {
		t.Fatalf("panic callback status %d: %s", rec.Code, rec.Body.String())
	}
}

func TestDebugExecAuditTimeout(t *testing.T) {
	d := &blockingDebugger{started: make(chan struct{}), release: make(chan struct{})}
	var events []debug.AuditEvent
	h := Handler(Opts{Prefix: "/apidoc", Store: newDebugStore(),
		Debuggers:        map[string]debug.Debugger{"mqtt": d},
		DebugAllowUnauth: true, DebugTimeout: 20 * time.Millisecond,
		DebugAudit: func(e debug.AuditEvent) { events = append(events, e) }})
	id := store.ActionID("a", "v1", "PUBLISH", "mqtt://broker/topic")
	body := fmt.Sprintf(`{"id":%q,"method":"PUBLISH","url":"mqtt://broker/topic",`+
		`"params":[],"body":"","encoding":"json"}`, id)
	if rec := debugPost(t, h, body, true); rec.Code != http.StatusGatewayTimeout {
		t.Fatalf("timeout status %d", rec.Code)
	}
	if len(events) != 1 || events[0].Status != auditTimeout {
		t.Fatalf("timeout audit = %+v", events)
	}
}

func TestDebugStreamAuditLifecycle(t *testing.T) {
	d := &fakeStreamDebugger{spec: streamSpec(), sess: newFakeStreamSession()}
	var events []debug.AuditEvent
	h := streamHandler(d, Opts{DebugAllowUnauth: true,
		DebugAudit: func(e debug.AuditEvent) { events = append(events, e) }})
	open := postDebug(t, h, "/api/debug/open", streamOpenBody(t), true)
	if open.Code != http.StatusOK {
		t.Fatalf("open status %d: %s", open.Code, open.Body.String())
	}
	sid := sessionIDFrom(t, open)
	send := fmt.Sprintf(`{"session_id":%q,"body":"d29ybGQ=","encoding":"text"}`, sid)
	if rec := postDebug(t, h, "/api/debug/send", send, true); rec.Code != http.StatusOK {
		t.Fatalf("send status %d: %s", rec.Code, rec.Body.String())
	}
	closeBody := fmt.Sprintf(`{"session_id":%q}`, sid)
	if rec := postDebug(t, h, "/api/debug/close", closeBody, true); rec.Code != http.StatusOK {
		t.Fatalf("close status %d: %s", rec.Code, rec.Body.String())
	}

	if len(events) != 3 {
		t.Fatalf("audit events = %d: %+v", len(events), events)
	}
	for _, ev := range events {
		if ev.ActionID != streamActionID() || ev.App != "a" || ev.Protocol != "ws" ||
			ev.Method != "CONNECT" || ev.Target != "ws://stream/topic" || ev.Status != auditOK {
			t.Fatalf("stream audit wrong: %+v", ev)
		}
	}
	if events[0].Bytes != 0 || events[1].Bytes != len("world") || events[2].Bytes != 0 {
		t.Fatalf("audit bytes wrong: %+v", events)
	}
}

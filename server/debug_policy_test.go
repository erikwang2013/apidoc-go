package server

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/erikwang2013/apidoc-go/debug"
	"github.com/erikwang2013/apidoc-go/debug/debugtest"
	"github.com/erikwang2013/apidoc-go/model"
	"github.com/erikwang2013/apidoc-go/store"
)

const policyTarget = "mqtt://127.0.0.1:1883/topic"

func newPolicyStore() *store.Store {
	s := store.New("p")
	s.Register("PUBLISH", policyTarget, model.Doc{
		App: "a", Version: "v1", Protocol: "mqtt", Title: "publish",
	})
	return s
}

func policyExecBody() string {
	return fmt.Sprintf(`{"id":%q,"method":"PUBLISH","url":%q,"params":[],`+
		`"body":"","encoding":"json"}`, store.ActionID("a", "v1", "PUBLISH", policyTarget), policyTarget)
}

func TestDebugExecPolicyDenied(t *testing.T) {
	d := &debugtest.FakeAdapter{Proto: "mqtt", SpecValue: debug.Spec{
		Mode: debug.ModeServer, Actions: []string{"PUBLISH"},
	}}
	var events []debug.AuditEvent
	h := Handler(Opts{Prefix: "/apidoc", Store: newPolicyStore(),
		Debuggers: map[string]debug.Debugger{"mqtt": d}, DebugAllowUnauth: true,
		DebugPolicy: &debug.Policy{AllowHosts: []string{"192.168.0.1"}},
		DebugAudit:  func(e debug.AuditEvent) { events = append(events, e) }})
	rec := debugPost(t, h, policyExecBody(), true)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	if len(d.Requests()) != 0 {
		t.Fatal("adapter ran despite denial")
	}
	if len(events) != 1 || events[0].Status != auditDenied || events[0].Error == "" {
		t.Fatalf("audit = %+v", events)
	}
}

func TestDebugExecPolicyAllowed(t *testing.T) {
	d := &debugtest.FakeAdapter{Proto: "mqtt", SpecValue: debug.Spec{
		Mode: debug.ModeServer, Actions: []string{"PUBLISH"},
	}}
	h := Handler(Opts{Prefix: "/apidoc", Store: newPolicyStore(),
		Debuggers: map[string]debug.Debugger{"mqtt": d}, DebugAllowUnauth: true,
		DebugPolicy: &debug.Policy{AllowHosts: []string{"127.0.0.1:1883"}}})
	if rec := debugPost(t, h, policyExecBody(), true); rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	if len(d.Requests()) != 1 {
		t.Fatalf("adapter calls = %d", len(d.Requests()))
	}
}

const policyStreamTarget = "ws://127.0.0.1:9000/topic"

func newPolicyStreamStore() *store.Store {
	s := store.New("p")
	s.Register("CONNECT", policyStreamTarget, model.Doc{
		App: "a", Version: "v1", Protocol: "ws", Title: "stream",
	})
	return s
}

func policyStreamBody() string {
	return fmt.Sprintf(`{"id":%q,"method":"CONNECT","url":%q,"encoding":"text"}`,
		store.ActionID("a", "v1", "CONNECT", policyStreamTarget), policyStreamTarget)
}

func TestDebugOpenPolicy(t *testing.T) {
	d := &fakeStreamDebugger{spec: streamSpec(), sess: newFakeStreamSession()}
	h := Handler(Opts{Prefix: "/apidoc", Store: newPolicyStreamStore(),
		Debuggers: map[string]debug.Debugger{"ws": d}, DebugAllowUnauth: true,
		DebugPolicy: &debug.Policy{AllowPorts: []int{9001}}})
	if rec := postDebug(t, h, "/api/debug/open", policyStreamBody(), true); rec.Code != http.StatusForbidden {
		t.Fatalf("denied open status = %d: %s", rec.Code, rec.Body.String())
	}
	if d.openCount() != 0 {
		t.Fatal("adapter opened despite denial")
	}

	h = Handler(Opts{Prefix: "/apidoc", Store: newPolicyStreamStore(),
		Debuggers: map[string]debug.Debugger{"ws": d}, DebugAllowUnauth: true,
		DebugPolicy: &debug.Policy{AllowHosts: []string{"127.0.0.1:9000"}}})
	if rec := postDebug(t, h, "/api/debug/open", policyStreamBody(), true); rec.Code != http.StatusOK {
		t.Fatalf("allowed open status = %d: %s", rec.Code, rec.Body.String())
	}
	if d.openCount() != 1 {
		t.Fatalf("adapter opens = %d", d.openCount())
	}
}

package apidoc

import (
	"net/http"
	"testing"

	"github.com/erikwang2013/apidoc-go/store"
)

type fakeFramework struct {
	registered []string
	mounted    bool
}

func (f *fakeFramework) Register(method, path string, h any) error {
	f.registered = append(f.registered, method+" "+path)
	return nil
}

func (f *fakeFramework) Mount(prefix string, h http.Handler) { f.mounted = true }

func TestNonHTTPRegisterDoesNotForward(t *testing.T) {
	ff := &fakeFramework{}
	s := New(Config{})
	if err := s.Mount(ff); err != nil {
		t.Fatal(err)
	}
	if err := s.Register(Route{
		Method: "SEND",
		URL:    "tcp://127.0.0.1:9000",
		Doc:    Doc{Protocol: "tcp", Title: "tcp echo"},
	}); err != nil {
		t.Fatal(err)
	}
	if len(ff.registered) != 0 {
		t.Fatalf("non-http route forwarded: %v", ff.registered)
	}
	a, ok := s.st.Action(store.ActionID("default", "v1", "SEND", "tcp://127.0.0.1:9000"))
	if !ok || a.Protocol != "tcp" {
		t.Fatalf("doc not stored: ok=%v action=%+v", ok, a)
	}
}

func TestNonHTTPPendingNotForwardedOnMount(t *testing.T) {
	s := New(Config{})
	if err := s.Register(Route{Method: "SEND", URL: "tcp://127.0.0.1:9000",
		Doc: Doc{Protocol: "tcp"}}); err != nil {
		t.Fatal(err)
	}
	ff := &fakeFramework{}
	if err := s.Mount(ff); err != nil {
		t.Fatal(err)
	}
	if len(ff.registered) != 0 {
		t.Fatalf("pending non-http route forwarded: %v", ff.registered)
	}
}

func TestHTTPStillForwards(t *testing.T) {
	s := New(Config{})
	ff := &fakeFramework{}
	if err := s.Mount(ff); err != nil {
		t.Fatal(err)
	}
	if err := s.Register(Route{Method: "GET", URL: "/ping",
		Handler: http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}),
		Doc:     Doc{Title: "ping"}}); err != nil {
		t.Fatal(err)
	}
	if len(ff.registered) != 1 || ff.registered[0] != "GET /ping" {
		t.Fatalf("http route not forwarded: %v", ff.registered)
	}
}

func TestDebuggerRegistryValidation(t *testing.T) {
	cases := []struct {
		name string
		cfg  Config
	}{
		{"duplicate", Config{Debuggers: []Debugger{
			DebuggerFunc("mqtt", DebugSpec{}, nil),
			DebuggerFunc("MQTT", DebugSpec{}, nil),
		}}},
		{"empty protocol", Config{Debuggers: []Debugger{
			DebuggerFunc("", DebugSpec{}, nil),
		}}},
		{"nil debugger", Config{Debuggers: []Debugger{nil}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatal("want panic")
				}
			}()
			New(c.cfg)
		})
	}
}

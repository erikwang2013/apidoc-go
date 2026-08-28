package store

import (
	"encoding/json"
	"testing"

	"github.com/erikwang2013/apidoc-go/model"
)

func TestRegisterDedupeLaterWins(t *testing.T) {
	s := New("p")
	first := model.Doc{App: "a", Version: "v1", Title: "first", Sort: 2,
		Params: []model.Param{{Name: "id", In: "query", Type: "int"}}}
	s.Register("GET", "/users", first)
	s.Register("GET", "/users", model.Doc{App: "a", Version: "v1", Title: "second", Sort: 1})

	id := ActionID("a", "v1", "GET", "/users")
	a, ok := s.Action(id)
	if !ok || a.Title != "second" || a.Sort != 1 || len(a.Params) != 0 {
		t.Fatalf("dedupe failed: ok=%v action=%+v", ok, a)
	}
	// exactly one action in the tree
	apps := s.Menus()
	if len(apps) != 1 || len(apps[0].Versions[0].Controllers[0].Actions) != 1 {
		t.Fatalf("expected single action in tree, got %+v", apps)
	}
	// different app / method / url are distinct endpoints
	s.Register("GET", "/users", model.Doc{App: "b", Version: "v1", Title: "other"})
	s.Register("POST", "/users", model.Doc{App: "a", Version: "v1", Title: "create"})
	if _, ok := s.Action(ActionID("b", "v1", "GET", "/users")); !ok {
		t.Fatal("app b action missing")
	}
	if _, ok := s.Action(ActionID("a", "v1", "POST", "/users")); !ok {
		t.Fatal("POST action missing")
	}
}

func TestJSONRoundTrip(t *testing.T) {
	s := New("p")
	s.SetAppMeta("a", "App A")
	s.Register("GET", "/users", model.Doc{App: "a", Version: "v1", Title: "list",
		Params: []model.Param{{Name: "id", In: "query", Type: "int"}}})
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	var s2 Store
	if err := json.Unmarshal(b, &s2); err != nil {
		t.Fatal(err)
	}
	id := ActionID("a", "v1", "GET", "/users")
	a, ok := s2.Action(id)
	if !ok || a.Title != "list" || len(a.Params) != 1 {
		t.Fatalf("round trip lost data: ok=%v action=%+v", ok, a)
	}
	// index rebuilt on load: dedupe still works afterwards
	s2.Register("GET", "/users", model.Doc{App: "a", Version: "v1", Title: "updated"})
	if a, _ := s2.Action(id); a.Title != "updated" {
		t.Fatalf("register after load failed: %+v", a)
	}
	if apps := s2.Menus(); len(apps[0].Versions[0].Controllers[0].Actions) != 1 {
		t.Fatal("round-trip store should still have one action")
	}
}

func TestDefaults(t *testing.T) {
	s := New("p")
	s.Register("GET", "/x", model.Doc{})
	a, ok := s.Action(ActionID("default", "v1", "GET", "/x"))
	if !ok || a.App != "default" || a.Version != "v1" || a.Controller != "default" {
		t.Fatalf("defaults not applied: ok=%v action=%+v", ok, a)
	}
}

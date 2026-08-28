package apidoc

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/erikwang2013/apidoc-go/store"
)

type createUserReq struct {
	Name   string `json:"name"`
	Age    int    `json:"age,omitempty"`
	secret string // unexported: skipped
	Addr   addr   `json:"addr"`
	Skip   string `json:"-"`
}

type addr struct {
	City string `json:"city"`
}

func TestReflectParamsStructArg(t *testing.T) {
	h := func(r *http.Request, req *createUserReq) {}
	ps := reflectParams(h)
	if len(ps) != 1 {
		t.Fatalf("want 1 param, got %d", len(ps))
	}
	p := ps[0]
	if p.Name != "" || p.In != "body" || p.Type != "createUserReq" {
		t.Fatalf("param shape wrong: %+v", p)
	}
	if len(p.Children) != 3 {
		t.Fatalf("want 3 children, got %+v", p.Children)
	}
	byName := map[string]Param{}
	for _, c := range p.Children {
		byName[c.Name] = c
	}
	if c := byName["name"]; c.Type != "string" {
		t.Fatalf("name child wrong: %+v", c)
	}
	if c := byName["age"]; c.Type != "int" {
		t.Fatalf("age child wrong: %+v", c)
	}
	if c := byName["addr"]; c.Type != "apidoc.addr" || len(c.Children) != 1 || c.Children[0].Name != "city" {
		t.Fatalf("addr child wrong: %+v", c)
	}
	if _, ok := byName["secret"]; ok {
		t.Fatal("unexported field leaked")
	}
	if _, ok := byName["Skip"]; ok {
		t.Fatal(`json:"-" field leaked`)
	}
}

func TestReflectParamsNoStructArgs(t *testing.T) {
	if ps := reflectParams(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})); ps != nil {
		t.Fatalf("handler without struct args: want nil, got %+v", ps)
	}
}

// The isCtx skip list is name-based: structs named like framework contexts
// are treated as contexts even when user-defined.
type Context struct{ Foo string }
type Request struct{ Foo string }

func TestReflectParamsSkipsContextNames(t *testing.T) {
	for _, h := range []any{func(c *Context) {}, func(r *Request) {}, func(ctx *Context, req *Request) {}} {
		if ps := reflectParams(h); ps != nil {
			t.Fatalf("context-named arg must be skipped, got %+v", ps)
		}
	}
}

func TestRegisterFillsDocParams(t *testing.T) {
	s := New(Config{})
	if err := s.Register(Route{Method: "POST", URL: "/users",
		Handler: func(r *http.Request, req *createUserReq) {},
		Doc:     Doc{App: "users", Version: "v1", Title: "create"}}); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet,
		"/api/detail?id="+store.ActionID("users", "v1", "POST", "/users"), nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("detail: want 200 got %d: %s", rec.Code, rec.Body.String())
	}
	var b struct {
		Data struct {
			Params []Param `json:"params"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &b); err != nil {
		t.Fatal(err)
	}
	ps := b.Data.Params
	if len(ps) != 1 || ps[0].In != "body" || ps[0].Type != "createUserReq" || len(ps[0].Children) != 3 {
		t.Fatalf("params not filled: %+v", ps)
	}
}

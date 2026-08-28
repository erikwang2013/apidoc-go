package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/erikwang2013/apidoc-go/auth"
	"github.com/erikwang2013/apidoc-go/model"
	"github.com/erikwang2013/apidoc-go/store"
)

func newTestStore() *store.Store {
	s := store.New("p")
	s.SetAppMeta("users", "Users")
	s.Register("GET", "/users", model.Doc{App: "users", Version: "v1", Title: "create user", Sort: 2})
	s.Register("GET", "/users/list", model.Doc{App: "users", Version: "v1", Title: "list", Sort: 1})
	s.Register("GET", "/ping", model.Doc{App: "users", Version: "v1", Title: "zero sort"})
	return s
}

func req(t *testing.T, h http.Handler, method, path, body string, c ...*http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	for _, ck := range c {
		r.AddCookie(ck)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	return rec
}

// menus DTO: exactly the fields /api/menus must expose.
type menusResp struct {
	Code int `json:"code"`
	Data []struct {
		Key      string `json:"key"`
		Title    string `json:"title"`
		Versions []struct {
			Name        string `json:"name"`
			Controllers []struct {
				Name    string `json:"name"`
				Actions []struct {
					ID    string `json:"id"`
					Title string `json:"title"`
					Sort  int    `json:"sort"`
				} `json:"actions"`
			} `json:"controllers"`
		} `json:"versions"`
	} `json:"data"`
}

func TestMenusTreeAndSort(t *testing.T) {
	h := Handler(Opts{Prefix: "/apidoc", Store: newTestStore()})
	rec := req(t, h, http.MethodGet, "/api/menus", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("menus status %d", rec.Code)
	}
	var b menusResp
	if err := json.Unmarshal(rec.Body.Bytes(), &b); err != nil {
		t.Fatal(err)
	}
	if len(b.Data) != 1 || b.Data[0].Key != "users" || b.Data[0].Title != "Users" {
		t.Fatalf("app shape wrong: %+v", b.Data)
	}
	app := b.Data[0]
	if len(app.Versions) != 1 || app.Versions[0].Name != "v1" {
		t.Fatalf("version shape wrong: %+v", app.Versions)
	}
	acts := app.Versions[0].Controllers[0].Actions
	if len(acts) != 3 {
		t.Fatalf("expected 3 actions, got %d", len(acts))
	}
	wantSort := []int{0, 1, 2}
	wantTitle := []string{"zero sort", "list", "create user"}
	for i, a := range acts {
		if a.Sort != wantSort[i] || a.Title != wantTitle[i] || a.ID == "" {
			t.Fatalf("action %d wrong: sort=%d title=%q id=%q", i, a.Sort, a.Title, a.ID)
		}
	}
}

func TestDetailMergesGlobalParams(t *testing.T) {
	s := store.New("p")
	s.Register("GET", "/users", model.Doc{App: "a", Version: "v1", Title: "t",
		Params:   []model.Param{{Name: "id", In: "query", Type: "int", Desc: "action id"}},
		Markdown: "**bold** <script>alert(1)</script>",
	})
	h := Handler(Opts{Prefix: "/apidoc", Store: s,
		GlobalParams: []model.Param{
			{Name: "token", In: "header", Type: "string", Desc: "global token"},
			{Name: "id", In: "query", Type: "string", Desc: "global id"},
		}})
	id := store.ActionID("a", "v1", "GET", "/users")
	rec := req(t, h, http.MethodGet, "/api/detail?id="+id, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("detail status %d: %s", rec.Code, rec.Body.String())
	}
	var b struct {
		Data struct {
			Method string `json:"method"`
			URL    string `json:"url"`
			Params []struct {
				Name, In, Type, Desc string
			} `json:"params"`
			MarkdownHTML string `json:"markdown_html"`
			Protected    bool   `json:"protected"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &b); err != nil {
		t.Fatal(err)
	}
	d := b.Data
	if d.Method != "GET" || d.URL != "/users" {
		t.Fatalf("method/url wrong: %+v", d)
	}
	// globals first, action param overrides the (name,in) match in place
	if len(d.Params) != 2 || d.Params[0].Name != "token" || d.Params[0].In != "header" {
		t.Fatalf("params wrong: %+v", d.Params)
	}
	if p := d.Params[1]; p.Name != "id" || p.In != "query" || p.Type != "int" || p.Desc != "action id" {
		t.Fatalf("override failed: %+v", p)
	}
	// goldmark defaults: bold rendered, raw script stripped
	if !strings.Contains(d.MarkdownHTML, "<strong>bold</strong>") ||
		strings.Contains(d.MarkdownHTML, "<script>") {
		t.Fatalf("markdown rendering wrong: %q", d.MarkdownHTML)
	}
	if d.Protected {
		t.Fatal("unprotected app marked protected")
	}
}

func TestAuthRequiredAndLoginFlow(t *testing.T) {
	h := Handler(Opts{Prefix: "/apidoc", Store: newTestStore(),
		Auth: auth.Config{Enable: true, Password: "pw", Secret: "sec", Expire: time.Hour}})
	if rec := req(t, h, http.MethodGet, "/api/menus", ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("menus without cookie: want 401 got %d", rec.Code)
	}
	if rec := req(t, h, http.MethodGet, "/api/detail?id=x", ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("detail without cookie: want 401 got %d", rec.Code)
	}
	// UI stays reachable so the login overlay can render
	if rec := req(t, h, http.MethodGet, "/", ""); rec.Code != http.StatusOK {
		t.Fatalf("UI with auth enabled: want 200 got %d", rec.Code)
	}
	if rec := req(t, h, http.MethodPost, "/api/login", `{"password":"wrong"}`); rec.Code != http.StatusUnauthorized {
		t.Fatalf("wrong password: want 401 got %d", rec.Code)
	}
	rec := req(t, h, http.MethodPost, "/api/login", `{"password":"pw"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("login: want 200 got %d: %s", rec.Code, rec.Body.String())
	}
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != cookieName {
		t.Fatalf("login did not set session cookie: %+v", cookies)
	}
	if rec := req(t, h, http.MethodGet, "/api/menus", "", cookies[0]); rec.Code != http.StatusOK {
		t.Fatalf("menus with cookie: want 200 got %d", rec.Code)
	}
	bad := *cookies[0]
	bad.Value = "tampered"
	if rec := req(t, h, http.MethodGet, "/api/menus", "", &bad); rec.Code != http.StatusUnauthorized {
		t.Fatalf("tampered cookie: want 401 got %d", rec.Code)
	}
}

func TestAppLoginScopedToken(t *testing.T) {
	s := store.New("p")
	s.Register("GET", "/admin", model.Doc{App: "admin", Version: "v1", Title: "secret"})
	// Global auth off so the scoped app-token gate is what's under test;
	// with Enable on, withAuth would 401 before the app-token check runs.
	h := Handler(Opts{Prefix: "/apidoc", Store: s,
		Auth:   auth.Config{Enable: false, Secret: "sec"},
		AppPWs: map[string]string{"admin": "apppw"},
	})
	id := store.ActionID("admin", "v1", "GET", "/admin")
	rec := req(t, h, http.MethodGet, "/api/detail?id="+id, "")
	if rec.Code != http.StatusUnauthorized || !strings.Contains(rec.Body.String(), "app token required") ||
		!strings.Contains(rec.Body.String(), "admin") {
		t.Fatalf("protected detail without token: want 401 naming app, got %d %s", rec.Code, rec.Body.String())
	}
	if rec := req(t, h, http.MethodPost, "/api/app-login", `{"app":"nope","password":"x"}`); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown app: want 404 got %d", rec.Code)
	}
	if rec := req(t, h, http.MethodPost, "/api/app-login", `{"app":"admin","password":"wrong"}`); rec.Code != http.StatusUnauthorized {
		t.Fatalf("wrong app password: want 401 got %d", rec.Code)
	}
	rec = req(t, h, http.MethodPost, "/api/app-login", `{"app":"admin","password":"apppw"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("app login: want 200 got %d: %s", rec.Code, rec.Body.String())
	}
	var b struct {
		Data struct {
			Token  string  `json:"token"`
			Expire float64 `json:"expire"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &b); err != nil || b.Data.Token == "" {
		t.Fatalf("app login response wrong: %s", rec.Body.String())
	}
	if b.Data.Expire != 3600 {
		t.Fatalf("app token TTL wrong: %v", b.Data.Expire)
	}
	req2 := httptest.NewRequest(http.MethodGet, "/api/detail?id="+id, nil)
	req2.Header.Set("X-Apidoc-App-Token", b.Data.Token)
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK || !strings.Contains(rec2.Body.String(), `"protected":true`) {
		t.Fatalf("detail with app token: want 200 protected, got %d %s", rec2.Code, rec2.Body.String())
	}
}

func TestCORSAllowedOriginsOnly(t *testing.T) {
	h := Handler(Opts{Prefix: "/apidoc", Store: newTestStore(),
		DebugOrigins: []string{"https://dev.example.com"}})
	pre := httptest.NewRequest(http.MethodOptions, "/api/menus", nil)
	pre.Header.Set("Origin", "https://dev.example.com")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, pre)
	if rec.Code != http.StatusNoContent || rec.Header().Get("Access-Control-Allow-Origin") != "https://dev.example.com" {
		t.Fatalf("allowed preflight: want 204 + ACAO, got %d", rec.Code)
	}
	pre.Header.Set("Origin", "https://evil.example.com")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, pre)
	if rec.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("disallowed origin got CORS headers")
	}
}

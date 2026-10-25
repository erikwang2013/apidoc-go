package uiapi

import (
	"bytes"
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

func newStore() *store.Store {
	st := store.New("Demo")
	st.SetAppMeta("default", "Default")
	st.Register("GET", "/items", model.Doc{
		App: "default", Version: "v1", Controller: "items", Title: "List items",
		Params: []model.Param{
			{Name: "page", In: "query", Type: "int", Required: false, Default: "1"},
			{Name: "X-Token", In: "header", Type: "string", Required: true},
		},
		Responses: []model.Response{
			{Name: "ok", Type: "object", Fields: []model.Param{{Name: "id", Type: "int", Required: true}}},
		},
		Markdown: "# Items",
	})
	st.Register("SUBSCRIBE", "ws://example.com/chat", model.Doc{
		App: "default", Version: "v1", Controller: "iot", Title: "Chat",
		Protocol: "ws",
		ProtocolOpts: map[string]string{
			"target": "ws://example.com/chat", "room": "lobby",
		},
		Events: []model.DocEvent{
			{Direction: "out", Name: "join", Type: "json", Desc: "join room", Mock: `{"room":"lobby"}`},
		},
	})
	return st
}

func newMultiAppStore() *store.Store {
	st := newStore()
	st.SetAppMeta("b", "B")
	st.Register("GET", "/b-items", model.Doc{
		App: "b", Version: "v1", Controller: "b", Title: "B items", Markdown: "# B",
	})
	return st
}

func newHandler(t *testing.T) http.Handler {
	t.Helper()
	mux := http.NewServeMux()
	Register(mux, Options{
		Store: newStore(),
		Auth: auth.Config{
			Enable: true, Password: "pw", Secret: "secret", Expire: time.Hour,
		},
	})
	return mux
}

func newHandlerWith(t *testing.T, o Options) http.Handler {
	t.Helper()
	if o.Store == nil {
		o.Store = newStore()
	}
	mux := http.NewServeMux()
	Register(mux, o)
	return mux
}

func do(t *testing.T, h http.Handler, method, path, body, token string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func decodeData(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var envelope struct {
		Code    int            `json:"code"`
		Message string         `json:"message"`
		Data    map[string]any `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode %s: %v", rec.Body.String(), err)
	}
	if envelope.Code != 0 {
		t.Fatalf("unexpected code %d: %s", envelope.Code, rec.Body.String())
	}
	return envelope.Data
}

func login(t *testing.T, h http.Handler) string {
	t.Helper()
	rec := do(t, h, "POST", "/ui/v1/verifyAuth", `{"password":"pw"}`, "")
	data := decodeData(t, rec)
	token, _ := data["token"].(string)
	if token == "" {
		t.Fatalf("no token: %s", rec.Body.String())
	}
	return token
}

func TestCapabilities(t *testing.T) {
	h := newHandler(t)
	rec := do(t, h, "GET", "/ui/v1/capabilities", "", "")
	data := decodeData(t, rec)
	if data["contract"] != "1.0" || data["backend"] != "go" {
		t.Fatalf("capabilities: %+v", data)
	}
}

func TestAuthAndConfig(t *testing.T) {
	h := newHandler(t)
	rec := do(t, h, "GET", "/ui/v1/config", "", "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
	token := login(t, h)
	rec = do(t, h, "GET", "/ui/v1/config", "", token)
	data := decodeData(t, rec)
	apps, _ := data["apps"].([]any)
	if len(apps) != 1 {
		t.Fatalf("apps: %+v", data["apps"])
	}
	app := apps[0].(map[string]any)
	if app["key"] != "default|v1" {
		t.Fatalf("app key: %+v", app)
	}
}

func TestMenusAndDetails(t *testing.T) {
	h := newHandler(t)
	token := login(t, h)
	rec := do(t, h, "POST", "/ui/v1/menus", `{"appKey":"default|v1"}`, token)
	data := decodeData(t, rec)
	controllers, _ := data["data"].([]any)
	if len(controllers) != 2 {
		t.Fatalf("controllers: %+v", data["data"])
	}

	httpID := store.ActionID("default", "v1", "GET", "/items")
	rec = do(t, h, "POST", "/ui/v1/detail", `{"appKey":"default|v1","path":"`+httpID+`"}`, token)
	detail := decodeData(t, rec)
	if detail["menuKey"] != httpID || detail["protocol"].(map[string]any)["name"] != "http" {
		t.Fatalf("http detail: %+v", detail)
	}
	if detail["debug"].(map[string]any)["mode"] != "browser-http" {
		t.Fatalf("http debug: %+v", detail["debug"])
	}

	wsID := store.ActionID("default", "v1", "SUBSCRIBE", "ws://example.com/chat")
	rec = do(t, h, "POST", "/ui/v1/detail", `{"appKey":"default|v1","path":"`+wsID+`"}`, token)
	ws := decodeData(t, rec)
	if ws["debug"].(map[string]any)["mode"] != "browser-websocket" {
		t.Fatalf("ws debug: %+v", ws["debug"])
	}
	defs, _ := ws["definitions"].([]any)
	if len(defs) != 1 {
		t.Fatalf("ws definitions: %+v", ws["definitions"])
	}
}

func TestDocs(t *testing.T) {
	h := newHandler(t)
	token := login(t, h)
	rec := do(t, h, "POST", "/ui/v1/docMenus", `{"appKey":"default|v1"}`, token)
	var menus struct {
		Code int   `json:"code"`
		Data []any `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &menus); err != nil || menus.Code != 0 || len(menus.Data) == 0 {
		t.Fatalf("doc menus: %s", rec.Body.String())
	}
	rec = do(t, h, "POST", "/ui/v1/docDetail", `{"appKey":"default|v1","path":"`+store.ActionID("default", "v1", "GET", "/items")+`"}`, token)
	doc := decodeData(t, rec)
	if !strings.Contains(doc["content"].(string), "# Items") {
		t.Fatalf("doc content: %+v", doc)
	}
}

func TestWrongPassword(t *testing.T) {
	h := newHandler(t)
	rec := do(t, h, "POST", "/ui/v1/verifyAuth", `{"password":"bad"}`, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func loginApp(t *testing.T, h http.Handler, appKey, password string) string {
	t.Helper()
	rec := do(t, h, "POST", "/ui/v1/verifyAuth",
		`{"password":"`+password+`","appKey":"`+appKey+`"}`, "")
	data := decodeData(t, rec)
	token, _ := data["token"].(string)
	if token == "" {
		t.Fatalf("no token: %s", rec.Body.String())
	}
	return token
}

func TestVerifyAuthShareKeyNotSupported(t *testing.T) {
	h := newHandler(t)
	rec := do(t, h, "POST", "/ui/v1/verifyAuth", `{"password":"pw","shareKey":"share-1"}`, "")
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("expected 501, got %d: %s", rec.Code, rec.Body.String())
	}
	var envelope struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil || envelope.Code != 501 {
		t.Fatalf("envelope: %s", rec.Body.String())
	}
}

// Global auth off plus app passwords must leave the app list public while
// app data still needs the matching app token.
func TestAppPasswordOnlyBootstrap(t *testing.T) {
	h := newHandlerWith(t, Options{
		Auth:   auth.Config{Secret: "secret"},
		AppPWs: map[string]string{"default": "apppw"},
	})

	rec := do(t, h, "GET", "/ui/v1/config", "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("public config: %d %s", rec.Code, rec.Body.String())
	}
	apps, _ := decodeData(t, rec)["apps"].([]any)
	if len(apps) != 1 {
		t.Fatalf("apps: %+v", apps)
	}

	rec = do(t, h, "POST", "/ui/v1/menus", `{"appKey":"default|v1"}`, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated menus: %d", rec.Code)
	}
	rec = do(t, h, "POST", "/ui/v1/verifyAuth", `{"password":"bad","appKey":"default|v1"}`, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("wrong app password: %d", rec.Code)
	}

	token := loginApp(t, h, "default|v1", "apppw")
	if scope, ok := auth.Verify("secret", token); !ok || scope != "default" {
		t.Fatalf("token scope: %q ok=%v", scope, ok)
	}
	rec = do(t, h, "POST", "/ui/v1/menus", `{"appKey":"default|v1"}`, token)
	if data := decodeData(t, rec); len(data["data"].([]any)) != 2 {
		t.Fatalf("menus: %+v", data["data"])
	}
	rec = do(t, h, "GET", "/ui/v1/docMenus?appKey=default%7Cv1", "", token)
	if len(decodeDataArray(t, rec)) == 0 {
		t.Fatalf("doc menus: %s", rec.Body.String())
	}
	id := store.ActionID("default", "v1", "GET", "/items")
	rec = do(t, h, "POST", "/ui/v1/docDetail", `{"appKey":"default|v1","path":"`+id+`"}`, token)
	if !strings.Contains(decodeData(t, rec)["content"].(string), "# Items") {
		t.Fatalf("doc detail: %s", rec.Body.String())
	}
}

func decodeDataArray(t *testing.T, rec *httptest.ResponseRecorder) []any {
	t.Helper()
	var envelope struct {
		Code int   `json:"code"`
		Data []any `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil || envelope.Code != 0 {
		t.Fatalf("decode %s: %v", rec.Body.String(), err)
	}
	return envelope.Data
}

// An app token is bound to its app: no other app's menus/docs, and an empty
// appKey is a missing parameter rather than a wildcard.
func TestAppScopeIsolation(t *testing.T) {
	h := newHandlerWith(t, Options{
		Store:  newMultiAppStore(),
		Auth:   auth.Config{Enable: true, Password: "pw", Secret: "secret", Expire: time.Hour},
		AppPWs: map[string]string{"default": "pw1", "b": "pw2"},
	})
	tokenA := loginApp(t, h, "default|v1", "pw1")
	idB := store.ActionID("b", "v1", "GET", "/b-items")
	bPath := `{"appKey":"b|v1","path":"` + idB + `"}`

	rec := do(t, h, "POST", "/ui/v1/menus", `{"appKey":"default|v1"}`, tokenA)
	if rec.Code != http.StatusOK {
		t.Fatalf("own app menus: %d", rec.Code)
	}
	for _, tc := range []struct {
		name, method, path, body string
	}{
		{"other app menus", "POST", "/ui/v1/menus", `{"appKey":"b|v1"}`},
		{"empty appKey menus", "POST", "/ui/v1/menus", `{}`},
		{"other app docMenus", "POST", "/ui/v1/docMenus", `{"appKey":"b|v1"}`},
		{"empty appKey docMenus", "POST", "/ui/v1/docMenus", `{}`},
		{"other app docDetail", "POST", "/ui/v1/docDetail", bPath},
	} {
		rec := do(t, h, tc.method, tc.path, tc.body, tokenA)
		want := http.StatusUnauthorized
		if strings.Contains(tc.name, "empty") {
			want = http.StatusBadRequest
		}
		if rec.Code != want {
			t.Fatalf("%s: got %d want %d (%s)", tc.name, rec.Code, want, rec.Body.String())
		}
	}

	rec = do(t, h, "GET", "/ui/v1/docMenus?appKey=b%7Cv1", "", tokenA)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("other app docMenus GET: %d", rec.Code)
	}
	rec = do(t, h, "POST", "/ui/v1/detail", bPath, tokenA)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("other app detail: %d", rec.Code)
	}
	rec = do(t, h, "GET", "/ui/v1/config", "", tokenA)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("empty appKey config with app token: %d", rec.Code)
	}
}

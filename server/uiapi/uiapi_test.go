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

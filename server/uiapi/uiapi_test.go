package uiapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
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
	return newHandlerWith(t, Options{
		Auth: auth.Config{
			Enable: true, Password: "pw", Secret: "secret", Expire: time.Hour,
		},
	})
}

func newHandlerWith(t *testing.T, o Options) http.Handler {
	t.Helper()
	if o.Store == nil {
		o.Store = newStore()
	}
	if o.ShareDir == "" {
		o.ShareDir = t.TempDir()
	}
	if o.Auth.Secret == "" {
		o.Auth.Secret = "secret"
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

func decodeDataOf[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var envelope struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    T      `json:"data"`
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
	features := data["features"].(map[string]any)
	share := features["share"].(map[string]any)
	if share["records"] != true || share["actions"] != false {
		t.Fatalf("unwired share features: %+v", share)
	}
	cache := features["cache"].(map[string]any)
	if cache["manage"] != false || cache["mode"] != "server" {
		t.Fatalf("unwired cache features: %+v", cache)
	}
	generator := features["generator"].(map[string]any)
	if generator["forms"] != true || generator["templates"] != true || generator["sideEffects"] != false {
		t.Fatalf("generator features: %+v", generator)
	}
	exports := features["exports"].([]any)
	for _, want := range []string{"md", "pdf", "openapi", "postman", "asyncapi", "typescript"} {
		found := false
		for _, got := range exports {
			if got == want {
				found = true
			}
		}
		if !found {
			t.Fatalf("missing export %q in %+v", want, exports)
		}
	}
	if features["auth"].(map[string]any)["shareToken"] != true {
		t.Fatalf("shareToken: %+v", features["auth"])
	}

	// Wired controls are advertised; unwired ones above are not.
	wired := newHandlerWith(t, Options{
		Auth:         auth.Config{Enable: true, Password: "pw", Secret: "secret", Expire: time.Hour},
		ShareActions: []ShareActionSpec{{Name: "Copy", Action: "copy"}},
		CacheClear:   func() {},
		CacheWarm:    func() {},
	})
	wiredFeatures := decodeData(t, do(t, wired, "GET", "/ui/v1/capabilities", "", ""))["features"].(map[string]any)
	if wiredFeatures["share"].(map[string]any)["actions"] != true ||
		wiredFeatures["cache"].(map[string]any)["manage"] != true {
		t.Fatalf("wired capabilities: %+v", wiredFeatures)
	}

	// One cache callback is not enough: the other control would answer 501.
	halfWired := newHandlerWith(t, Options{
		Auth:       auth.Config{Enable: true, Password: "pw", Secret: "secret", Expire: time.Hour},
		CacheClear: func() {},
	})
	halfFeatures := decodeData(t, do(t, halfWired, "GET", "/ui/v1/capabilities", "", ""))["features"].(map[string]any)
	if halfFeatures["cache"].(map[string]any)["manage"] != false {
		t.Fatalf("half-wired cache capabilities: %+v", halfFeatures)
	}
}

// Auth capabilities must mirror what verifyAuth actually accepts: Enable
// without a password fails closed, and app tokens need a non-empty app
// password.
func TestAuthCapabilities(t *testing.T) {
	cases := []struct {
		name       string
		auth       auth.Config
		appPWs     map[string]string
		wantGlobal bool
		wantApp    bool
	}{
		{"off", auth.Config{}, nil, false, false},
		{"enable without password", auth.Config{Enable: true}, nil, false, false},
		{"password without enable", auth.Config{Password: "pw"}, nil, false, false},
		{"global on", auth.Config{Enable: true, Password: "pw"}, nil, true, false},
		{"empty app password", auth.Config{Enable: true, Password: "pw"}, map[string]string{"a": ""}, true, false},
		{"app password", auth.Config{Enable: true, Password: "pw"}, map[string]string{"a": "", "b": "bpw"}, true, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := newHandlerWith(t, Options{Auth: c.auth, AppPWs: c.appPWs})
			features := decodeData(t, do(t, h, "GET", "/ui/v1/capabilities", "", ""))["features"].(map[string]any)
			authFeatures := features["auth"].(map[string]any)
			if authFeatures["global"] != c.wantGlobal || authFeatures["app"] != c.wantApp {
				t.Fatalf("auth capabilities: want global=%v app=%v, got %+v",
					c.wantGlobal, c.wantApp, authFeatures)
			}
			if authFeatures["shareToken"] != true {
				t.Fatalf("shareToken: %+v", authFeatures)
			}
		})
	}
}

// The Go capabilities block must stay in sync with the shared contract fixture.
func TestCapabilitiesMatchGoFixture(t *testing.T) {
	b, err := os.ReadFile("../../contract/uic-1/fixtures/capabilities.json")
	if err != nil {
		t.Fatal(err)
	}
	var rows []struct {
		Response struct {
			Data map[string]any `json:"data"`
		} `json:"response"`
	}
	if err := json.Unmarshal(b, &rows); err != nil {
		t.Fatal(err)
	}
	var want any
	for _, row := range rows {
		if row.Response.Data["backend"] == "go" {
			want = row.Response.Data["features"]
			break
		}
	}
	if want == nil {
		t.Fatal("no go capabilities fixture")
	}
	// The fixture describes a server with global auth, an app password,
	// share actions and both cache callbacks wired, so compare against that
	// configuration.
	live := decodeData(t, do(t, newHandlerWith(t, Options{
		Auth:         auth.Config{Enable: true, Password: "pw", Secret: "secret", Expire: time.Hour},
		AppPWs:       map[string]string{"default": "apppw"},
		ShareActions: []ShareActionSpec{{Name: "Copy", Action: "copy"}},
		CacheClear:   func() {},
		CacheWarm:    func() {},
	}), http.MethodGet, "/ui/v1/capabilities", "", ""))["features"]
	if liveFeatures, ok := live.(map[string]any); ok {
		if liveDebug, ok := liveFeatures["debug"].(map[string]any); ok {
			if wantDebug, ok := want.(map[string]any)["debug"].(map[string]any); ok {
				// The shared fixture pins the default-build surface; tagged
				// builds compare against their compiled registry and modes.
				wantDebug["relay"] = fixtureRelay(liveDebug["relay"], wantDebug["relay"])
				wantDebug["modes"] = fixtureModes(liveDebug["modes"], wantDebug["modes"])
			}
		}
	}
	if !reflect.DeepEqual(jsonRoundTrip(t, live), jsonRoundTrip(t, want)) {
		t.Fatalf("capabilities drift:\nlive %#v\nwant %#v", live, want)
	}
}

func jsonRoundTrip(t *testing.T, v any) any {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var out any
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestHTTPRelayResponseEncoding(t *testing.T) {
	st := store.New("Debug")
	st.SetAppMeta("default", "Default")
	st.Register("POST", "https://example.com/graphql", model.Doc{
		App: "default", Version: "v1", Controller: "debug", Title: "GraphQL",
		Protocol: "graphql",
	})
	id := store.ActionID("default", "v1", "POST", "https://example.com/graphql")
	h := newHandlerWith(t, Options{Store: st})
	data := decodeData(t, do(t, h, "POST", "/ui/v1/detail", `{"path":"`+id+`"}`, ""))
	debug := data["debug"].(map[string]any)
	if debug["responseEncoding"] != "json" || data["protocol"].(map[string]any)["http"] != false {
		t.Fatalf("graphql debug metadata: %+v", debug)
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

func TestVerifyAuthUnknownShare(t *testing.T) {
	h := newHandler(t)
	rec := do(t, h, "POST", "/ui/v1/verifyAuth", `{"password":"pw","shareKey":"share-1"}`, "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
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

// Mixed deployment: global auth off, one app protected and one not. The app
// list and the unprotected app stay public; only the protected app needs its
// token.
func TestMixedAppPasswordPublicApp(t *testing.T) {
	h := newHandlerWith(t, Options{
		Store:  newMultiAppStore(),
		Auth:   auth.Config{Secret: "secret"},
		AppPWs: map[string]string{"default": "pw1"},
	})

	rec := do(t, h, "GET", "/ui/v1/config", "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("public config: %d %s", rec.Code, rec.Body.String())
	}
	if apps, _ := decodeData(t, rec)["apps"].([]any); len(apps) != 2 {
		t.Fatalf("apps: %+v", apps)
	}

	rec = do(t, h, "POST", "/ui/v1/menus", `{"appKey":"b|v1"}`, "")
	if data := decodeData(t, rec); len(data["data"].([]any)) != 1 {
		t.Fatalf("public app menus: %+v", data["data"])
	}
	rec = do(t, h, "POST", "/ui/v1/docMenus", `{"appKey":"b|v1"}`, "")
	if len(decodeDataArray(t, rec)) == 0 {
		t.Fatalf("public app doc menus: %s", rec.Body.String())
	}
	idB := store.ActionID("b", "v1", "GET", "/b-items")
	rec = do(t, h, "POST", "/ui/v1/docDetail", `{"appKey":"b|v1","path":"`+idB+`"}`, "")
	if !strings.Contains(decodeData(t, rec)["content"].(string), "# B") {
		t.Fatalf("public app doc detail: %s", rec.Body.String())
	}

	rec = do(t, h, "POST", "/ui/v1/menus", `{"appKey":"default|v1"}`, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("protected app without token: %d", rec.Code)
	}
	token := loginApp(t, h, "default|v1", "pw1")
	rec = do(t, h, "POST", "/ui/v1/menus", `{"appKey":"default|v1"}`, token)
	if data := decodeData(t, rec); len(data["data"].([]any)) != 2 {
		t.Fatalf("protected app menus: %+v", data["data"])
	}
	rec = do(t, h, "POST", "/ui/v1/menus", `{}`, token)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("empty appKey menus with app token: %d", rec.Code)
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
func TestShareCRUD(t *testing.T) {
	h := newHandlerWith(t, Options{
		Auth: auth.Config{Enable: true, Password: "pw", Secret: "secret", Expire: time.Hour},
		ShareActions: []ShareActionSpec{{
			Name: "Copy link", Action: "copy",
			Params: map[string]any{"url": "https://api.example.com/apidoc?app=default"},
		}},
	})
	token := login(t, h)

	rec := do(t, h, "POST", "/ui/v1/addApiShare", `{"name":"Team","type":"app","appKeys":["default"]}`, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated add: %d", rec.Code)
	}
	rec = do(t, h, "POST", "/ui/v1/addApiShare",
		`{"key":"share-1","name":"Team","type":"app","appKeys":["default"],"password":"secret"}`, token)
	if !decodeDataOf[bool](t, rec) {
		t.Fatalf("add: %s", rec.Body.String())
	}
	rec = do(t, h, "POST", "/ui/v1/addApiShare", `{"key":"../evil","name":"x","type":"all"}`, token)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("traversal key: %d %s", rec.Code, rec.Body.String())
	}
	rec = do(t, h, "POST", "/ui/v1/addApiShare", `{"name":"x","type":"api"}`, token)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("api share without apiKeys: %d", rec.Code)
	}

	rec = do(t, h, "POST", "/ui/v1/getApiShareList", `{"pageIndex":1}`, token)
	if strings.Contains(rec.Body.String(), "password") || strings.Contains(rec.Body.String(), "secret") {
		t.Fatalf("list leaks credentials: %s", rec.Body.String())
	}
	list := decodeData(t, rec)
	if list["total"].(float64) != 1 {
		t.Fatalf("list total: %+v", list)
	}
	item := list["data"].([]any)[0].(map[string]any)
	if item["key"] != "share-1" || item["name"] != "Team" || item["type"] != "app" {
		t.Fatalf("list item: %+v", item)
	}
	if item["create_at"] == "" || item["create_at"] != item["create_time"] {
		t.Fatalf("create times: %+v", item)
	}
	if keys, _ := item["appKeys"].([]any); len(keys) != 1 || keys[0] != "default" {
		t.Fatalf("appKeys: %+v", item["appKeys"])
	}

	rec = do(t, h, "POST", "/ui/v1/getApiShareDetail", `{"key":"share-1"}`, token)
	if strings.Contains(rec.Body.String(), "password") || strings.Contains(rec.Body.String(), "secret") {
		t.Fatalf("detail leaks credentials: %s", rec.Body.String())
	}
	detail := decodeData(t, rec)
	if detail["key"] != "share-1" || detail["name"] != "Team" {
		t.Fatalf("detail: %+v", detail)
	}

	rec = do(t, h, "POST", "/ui/v1/addApiShare", `{"key":"share-1","name":"Team 2","type":"all"}`, token)
	if !decodeDataOf[bool](t, rec) {
		t.Fatalf("edit: %s", rec.Body.String())
	}
	detail = decodeData(t, do(t, h, "POST", "/ui/v1/getApiShareDetail", `{"key":"share-1"}`, token))
	if detail["name"] != "Team 2" || detail["type"] != "all" {
		t.Fatalf("edited detail: %+v", detail)
	}
	// An edit without a password keeps the old one.
	rec = do(t, h, "POST", "/ui/v1/verifyAuth", `{"shareKey":"share-1","password":"secret"}`, "")
	if decodeData(t, rec)["token"] == "" {
		t.Fatalf("share login after edit: %s", rec.Body.String())
	}
	rec = do(t, h, "POST", "/ui/v1/verifyAuth", `{"shareKey":"share-1","password":"bad"}`, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("share wrong password: %d", rec.Code)
	}

	action := decodeData(t, do(t, h, "POST", "/ui/v1/handleApiShareAction", `{"key":"share-1","index":0}`, token))
	if action["action"] != "copy" ||
		action["params"].(map[string]any)["url"] != "https://api.example.com/apidoc?app=default" {
		t.Fatalf("share action: %+v", action)
	}
	if got := decodeDataOf[any](t, do(t, h, "POST", "/ui/v1/handleApiShareAction", `{"key":"share-1","index":9}`, token)); got != false {
		t.Fatalf("out of range action: %+v", got)
	}

	if !decodeDataOf[bool](t, do(t, h, "POST", "/ui/v1/deleteApiShare", `{"key":"share-1"}`, token)) {
		t.Fatal("delete failed")
	}
	rec = do(t, h, "POST", "/ui/v1/getApiShareDetail", `{"key":"share-1"}`, token)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("deleted detail: %d", rec.Code)
	}
	if total := decodeData(t, do(t, h, "POST", "/ui/v1/getApiShareList", `{}`, token))["total"]; total.(float64) != 0 {
		t.Fatalf("list after delete: %+v", total)
	}
}

func TestShareLoginAndScope(t *testing.T) {
	h := newHandlerWith(t, Options{
		Store:  newMultiAppStore(),
		Auth:   auth.Config{Enable: true, Password: "pw", Secret: "secret", Expire: time.Hour},
		AppPWs: map[string]string{"default": "apppw", "b": "apppw2"},
	})
	admin := login(t, h)
	itemsID := store.ActionID("default", "v1", "GET", "/items")
	wsID := store.ActionID("default", "v1", "SUBSCRIBE", "ws://example.com/chat")

	for _, body := range []string{
		`{"key":"s1","name":"Default share","type":"app","appKeys":["default"],"password":"sp"}`,
		`{"key":"open","name":"Open","type":"app","appKeys":["default"]}`,
		`{"key":"s2","name":"One API","type":"api","appKeys":["default"],"apiKeys":["` + itemsID + `"],"password":"sp"}`,
	} {
		rec := do(t, h, "POST", "/ui/v1/addApiShare", body, admin)
		if !decodeDataOf[bool](t, rec) {
			t.Fatalf("add %s: %s", body, rec.Body.String())
		}
	}

	rec := do(t, h, "POST", "/ui/v1/verifyAuth", `{"shareKey":"s1","password":"bad"}`, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("wrong share password: %d", rec.Code)
	}
	shareToken := decodeData(t, do(t, h, "POST", "/ui/v1/verifyAuth", `{"shareKey":"s1","password":"sp"}`, ""))["token"].(string)
	apiToken := decodeData(t, do(t, h, "POST", "/ui/v1/verifyAuth", `{"shareKey":"s2","password":"sp"}`, ""))["token"].(string)
	if shareToken == "" || apiToken == "" {
		t.Fatal("empty share token")
	}

	// Share config is scoped to the share's apps.
	rec = do(t, h, "GET", "/ui/v1/config?shareKey=s1", "", shareToken)
	apps := decodeData(t, rec)["apps"].([]any)
	if len(apps) != 1 || apps[0].(map[string]any)["key"] != "default|v1" {
		t.Fatalf("share config apps: %+v", apps)
	}
	if rec = do(t, h, "GET", "/ui/v1/config?shareKey=s1", "", ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("share config without token: %d", rec.Code)
	}
	// A share token is only valid alongside its shareKey, and an app token
	// cannot impersonate a share.
	if rec = do(t, h, "POST", "/ui/v1/menus", `{"appKey":"default|v1"}`, shareToken); rec.Code != http.StatusUnauthorized {
		t.Fatalf("share token without shareKey: %d", rec.Code)
	}
	appToken := loginApp(t, h, "default|v1", "apppw")
	if rec = do(t, h, "POST", "/ui/v1/menus", `{"appKey":"default|v1","shareKey":"s1"}`, appToken); rec.Code != http.StatusUnauthorized {
		t.Fatalf("app token with shareKey: %d", rec.Code)
	}
	if rec = do(t, h, "POST", "/ui/v1/menus", `{"appKey":"b|v1","shareKey":"s1"}`, shareToken); rec.Code != http.StatusNotFound {
		t.Fatalf("outside share app: %d %s", rec.Code, rec.Body.String())
	}

	// An app-wide share sees every API of its app despite the app password.
	rec = do(t, h, "POST", "/ui/v1/menus", `{"appKey":"default|v1","shareKey":"s1"}`, shareToken)
	if data := decodeData(t, rec); len(data["data"].([]any)) != 2 {
		t.Fatalf("share menus: %+v", data["data"])
	}
	rec = do(t, h, "POST", "/ui/v1/detail", `{"appKey":"default|v1","path":"`+wsID+`","shareKey":"s1"}`, shareToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("share detail: %d %s", rec.Code, rec.Body.String())
	}

	// A passwordless share works without a token.
	rec = do(t, h, "GET", "/ui/v1/config?shareKey=open", "", "")
	if apps := decodeData(t, rec)["apps"].([]any); len(apps) != 1 {
		t.Fatalf("open share config: %+v", apps)
	}
	rec = do(t, h, "POST", "/ui/v1/menus", `{"appKey":"default|v1","shareKey":"open"}`, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("open share menus: %d %s", rec.Code, rec.Body.String())
	}
	// A public share cannot be logged into: reads need no token, so any or
	// empty password is a password error (PHP parity).
	rec = do(t, h, "POST", "/ui/v1/verifyAuth", `{"shareKey":"open","password":""}`, "")
	if rec.Code != http.StatusUnauthorized || !strings.Contains(rec.Body.String(), "password error") {
		t.Fatalf("public share login: %d %s", rec.Code, rec.Body.String())
	}

	// An API-scoped share only exposes the selected menuKey.
	rec = do(t, h, "POST", "/ui/v1/menus", `{"appKey":"default|v1","shareKey":"s2"}`, apiToken)
	data := decodeData(t, rec)
	controllers := data["data"].([]any)
	if len(controllers) != 1 {
		t.Fatalf("api share controllers: %+v", controllers)
	}
	children := controllers[0].(map[string]any)["children"].([]any)
	if len(children) != 1 || children[0].(map[string]any)["menuKey"] != itemsID {
		t.Fatalf("api share actions: %+v", children)
	}
	rec = do(t, h, "POST", "/ui/v1/detail", `{"appKey":"default|v1","path":"`+wsID+`","shareKey":"s2"}`, apiToken)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("share api outside scope: %d", rec.Code)
	}
	rec = do(t, h, "POST", "/ui/v1/detail", `{"appKey":"default|v1","path":"`+itemsID+`","shareKey":"s2"}`, apiToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("share api inside scope: %d %s", rec.Code, rec.Body.String())
	}
	rec = do(t, h, "POST", "/ui/v1/docMenus", `{"appKey":"default|v1","shareKey":"s2"}`, apiToken)
	docControllers := decodeDataArray(t, rec)
	if len(docControllers) != 1 {
		t.Fatalf("api share docs: %+v", docControllers)
	}

	// exportAllData {key} is owner-only and honours the share scope.
	if rec = do(t, h, "POST", "/ui/v1/exportAllData", `{"key":"s2"}`, ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("share export without admin: %d", rec.Code)
	}
	rec = do(t, h, "POST", "/ui/v1/exportAllData", `{"key":"s2"}`, admin)
	apiData := decodeData(t, rec)["apiData"].([]any)
	if len(apiData) != 1 {
		t.Fatalf("share export apps: %+v", apiData)
	}
	if leaves := exportLeafKeys(apiData); len(leaves) != 1 || leaves[0] != itemsID {
		t.Fatalf("share export leaves: %+v", leaves)
	}
	rec = do(t, h, "POST", "/ui/v1/exportAllData", `{"key":"s1"}`, admin)
	if leaves := exportLeafKeys(decodeData(t, rec)["apiData"].([]any)); len(leaves) != 2 {
		t.Fatalf("app share export leaves: %+v", leaves)
	}

	// App tokens still cannot cross apps.
	if rec = do(t, h, "POST", "/ui/v1/exportAllData", `{"appKey":"b|v1"}`, appToken); rec.Code != http.StatusUnauthorized {
		t.Fatalf("cross-app export: %d", rec.Code)
	}
	rec = do(t, h, "POST", "/ui/v1/exportAllData", `{"appKey":"default|v1"}`, appToken)
	if leaves := exportLeafKeys(decodeData(t, rec)["apiData"].([]any)); len(leaves) != 2 {
		t.Fatalf("app export leaves: %+v", leaves)
	}
}

func TestCacheManagement(t *testing.T) {
	cleared, warmed := 0, 0
	h := newHandlerWith(t, Options{
		Auth:       auth.Config{Enable: true, Password: "pw", Secret: "secret", Expire: time.Hour},
		CacheClear: func() { cleared++ },
		CacheWarm:  func() { warmed++ },
	})
	if rec := do(t, h, "POST", "/ui/v1/cache/clear", `{}`, ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated cache clear: %d", rec.Code)
	}
	token := login(t, h)
	clear := decodeData(t, do(t, h, "POST", "/ui/v1/cache/clear", `{}`, token))
	if clear["ok"] != true || clear["mode"] != "server" {
		t.Fatalf("cache clear: %+v", clear)
	}
	warm := decodeData(t, do(t, h, "POST", "/ui/v1/cache/warm", `{}`, token))
	if warm["ok"] != true || warm["mode"] != "server" || warm["message"] != "cache rebuilt" {
		t.Fatalf("cache warm: %+v", warm)
	}
	if cleared != 1 || warmed != 1 {
		t.Fatalf("callbacks: cleared=%d warmed=%d", cleared, warmed)
	}
	config := decodeData(t, do(t, h, "GET", "/ui/v1/config", "", token))
	if config["cache"].(map[string]any)["enable"] != true {
		t.Fatalf("cache config: %+v", config["cache"])
	}
}

func TestGeneratorEndpoints(t *testing.T) {
	h := newHandlerWith(t, Options{
		Auth: auth.Config{Enable: true, Password: "pw", Secret: "secret", Expire: time.Hour},
		Generator: []GeneratorSpec{{
			Name: "model", Title: "Model",
			Files: []GeneratorFile{{
				Name: "User", Path: "src/User.ts", Template: "export interface {{.name}} {}\n",
			}},
			Form: map[string]any{"items": []any{}},
		}},
		CodeTemplates: []CodeTemplateSpec{{
			Title: "API client", Template: "api.ts", Content: "// {{.appKey}} {{len .apis}}\n",
			SelectMode: "api", Multiple: true,
		}},
	})
	if rec := do(t, h, "POST", "/ui/v1/generator", `{"index":0}`, ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated generator: %d", rec.Code)
	}
	token := login(t, h)

	rec := do(t, h, "POST", "/ui/v1/generator",
		`{"index":0,"form":{"name":"User"},"files":[{"name":"User","path":"gen/User.ts"}],"tables":[]}`, token)
	data := decodeData(t, rec)
	files := data["files"].([]any)
	if len(files) != 1 {
		t.Fatalf("generator files: %+v", files)
	}
	file := files[0].(map[string]any)
	if file["name"] != "User" || file["path"] != "gen/User.ts" ||
		file["content"] != "export interface User {}\n" {
		t.Fatalf("generator file: %+v", file)
	}
	if rec = do(t, h, "POST", "/ui/v1/generator", `{"index":9}`, token); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown generator: %d", rec.Code)
	}

	rec = do(t, h, "GET", "/ui/v1/config", "", token)
	config := decodeData(t, rec)
	generators := config["generator"].([]any)
	templates := config["code_template"].([]any)
	if len(generators) != 1 || len(templates) != 1 {
		t.Fatalf("config descriptors: %+v", config)
	}
	if strings.Contains(rec.Body.String(), "// {{.appKey}}") {
		t.Fatalf("code template source leaked: %s", rec.Body.String())
	}

	itemsID := store.ActionID("default", "v1", "GET", "/items")
	rec = do(t, h, "POST", "/ui/v1/renderCodeTemplate",
		`{"appKey":"default|v1","template":"api.ts","selected":["`+itemsID+`"],"form":{}}`, token)
	if code := decodeData(t, rec)["code"]; code != "// default|v1 1\n" {
		t.Fatalf("code template: %+v", code)
	}
	rec = do(t, h, "POST", "/ui/v1/renderCodeTemplate",
		`{"appKey":"default|v1","template":{"title":"API client","template":"api.ts"},"selected":["`+itemsID+`"],"form":{}}`, token)
	if code := decodeData(t, rec)["code"]; code != "// default|v1 1\n" {
		t.Fatalf("object code template: %+v", code)
	}
	if rec = do(t, h, "POST", "/ui/v1/renderCodeTemplate",
		`{"appKey":"default|v1","template":"nope.ts","selected":[],"form":{}}`, token); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown code template: %d", rec.Code)
	}
}

func TestExportAllData(t *testing.T) {
	h := newHandlerWith(t, Options{Store: newMultiAppStore()})
	itemsID := store.ActionID("default", "v1", "GET", "/items")
	wsID := store.ActionID("default", "v1", "SUBSCRIBE", "ws://example.com/chat")

	rec := do(t, h, "POST", "/ui/v1/exportAllData", `{"appKey":"default|v1"}`, "")
	data := decodeData(t, rec)
	apiData := data["apiData"].([]any)
	if len(apiData) != 1 {
		t.Fatalf("export apps: %+v", apiData)
	}
	node := apiData[0].(map[string]any)
	if node["key"] != "default" || node["appKey"] != "default|v1" || node["menuKey"] != "default|v1" {
		t.Fatalf("export node: %+v", node)
	}
	leaves := exportLeafKeys(apiData)
	if len(leaves) != 2 || leaves[0] != itemsID || leaves[1] != wsID {
		t.Fatalf("export leaves: %+v", leaves)
	}
	leaf := findExportLeaf(t, apiData, itemsID)
	if leaf["protocol"].(map[string]any)["name"] != "http" ||
		leaf["query"].([]any)[0].(map[string]any)["name"] != "page" {
		t.Fatalf("export detail mapping: %+v", leaf)
	}
	ws := findExportLeaf(t, apiData, wsID)
	if ws["debug"].(map[string]any)["mode"] != "browser-websocket" {
		t.Fatalf("export ws detail: %+v", ws["debug"])
	}

	if got := decodeData(t, do(t, h, "POST", "/ui/v1/exportAllData", `{"appKey":"default"}`, ""))["apiData"].([]any); len(got) != 1 {
		t.Fatalf("bare app export: %+v", got)
	}
	if got := decodeData(t, do(t, h, "POST", "/ui/v1/exportAllData", `{}`, ""))["apiData"].([]any); len(got) != 2 {
		t.Fatalf("all app export: %+v", got)
	}

	h2 := newHandlerWith(t, Options{
		Store:  newMultiAppStore(),
		Auth:   auth.Config{Secret: "secret"},
		AppPWs: map[string]string{"default": "pw1", "b": "pw2"},
	})
	appToken := loginApp(t, h2, "default|v1", "pw1")
	if rec := do(t, h2, "POST", "/ui/v1/exportAllData", `{"appKey":"default|v1"}`, ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("protected export without token: %d", rec.Code)
	}
	rec = do(t, h2, "POST", "/ui/v1/exportAllData", `{"appKey":"default|v1"}`, appToken)
	if got := decodeData(t, rec)["apiData"].([]any); len(got) != 1 {
		t.Fatalf("app token export: %+v", got)
	}
	if rec = do(t, h2, "POST", "/ui/v1/exportAllData", `{"appKey":"b|v1"}`, appToken); rec.Code != http.StatusUnauthorized {
		t.Fatalf("cross-app export: %d", rec.Code)
	}
}

// Global auth off: an anonymous export may only see unprotected apps, while a
// named protected app and password-protected shares still need their token.
func TestExportAllDataMixedAuth(t *testing.T) {
	h := newHandlerWith(t, Options{
		Store:  newMultiAppStore(),
		Auth:   auth.Config{Secret: "secret"},
		AppPWs: map[string]string{"default": "pw1"},
	})
	itemsID := store.ActionID("default", "v1", "GET", "/items")
	wsID := store.ActionID("default", "v1", "SUBSCRIBE", "ws://example.com/chat")
	bID := store.ActionID("b", "v1", "GET", "/b-items")
	appToken := loginApp(t, h, "default|v1", "pw1")

	// Empty body: only the unprotected app is exported.
	apiData := decodeData(t, do(t, h, "POST", "/ui/v1/exportAllData", `{}`, ""))["apiData"].([]any)
	if len(apiData) != 1 || apiData[0].(map[string]any)["appKey"] != "b|v1" {
		t.Fatalf("anonymous all-apps export: %+v", apiData)
	}
	if leaves := exportLeafKeys(apiData); len(leaves) != 1 || leaves[0] != bID {
		t.Fatalf("anonymous export leaves: %+v", leaves)
	}

	// Named apps: the protected one needs its own token, the public one stays
	// open.
	if rec := do(t, h, "POST", "/ui/v1/exportAllData", `{"appKey":"default|v1"}`, ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("protected export without token: %d", rec.Code)
	}
	rec := do(t, h, "POST", "/ui/v1/exportAllData", `{"appKey":"default|v1"}`, appToken)
	if leaves := exportLeafKeys(decodeData(t, rec)["apiData"].([]any)); len(leaves) != 2 || leaves[0] != itemsID || leaves[1] != wsID {
		t.Fatalf("protected export leaves: %+v", leaves)
	}
	if rec = do(t, h, "POST", "/ui/v1/exportAllData", `{"appKey":"b|v1"}`, ""); rec.Code != http.StatusOK {
		t.Fatalf("public app export: %d", rec.Code)
	}

	// Shares: management is open with global auth off, so seed both records
	// anonymously. A password-protected share still needs its share token.
	for _, body := range []string{
		`{"key":"s-pw","name":"Protected","type":"app","appKeys":["default"],"password":"sp"}`,
		`{"key":"s-open","name":"Open","type":"app","appKeys":["b"]}`,
	} {
		if !decodeDataOf[bool](t, do(t, h, "POST", "/ui/v1/addApiShare", body, "")) {
			t.Fatalf("add %s", body)
		}
	}
	rec = do(t, h, "POST", "/ui/v1/exportAllData", `{"key":"s-open"}`, "")
	if leaves := exportLeafKeys(decodeData(t, rec)["apiData"].([]any)); len(leaves) != 1 || leaves[0] != bID {
		t.Fatalf("open share export leaves: %+v", leaves)
	}
	if rec = do(t, h, "POST", "/ui/v1/exportAllData", `{"key":"s-pw"}`, ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("password share export without token: %d", rec.Code)
	}
	if rec = do(t, h, "POST", "/ui/v1/exportAllData", `{"key":"s-pw"}`, appToken); rec.Code != http.StatusUnauthorized {
		t.Fatalf("password share export with app token: %d", rec.Code)
	}
	shareToken := decodeData(t, do(t, h, "POST", "/ui/v1/verifyAuth", `{"shareKey":"s-pw","password":"sp"}`, ""))["token"].(string)
	rec = do(t, h, "POST", "/ui/v1/exportAllData", `{"key":"s-pw"}`, shareToken)
	if leaves := exportLeafKeys(decodeData(t, rec)["apiData"].([]any)); len(leaves) != 2 {
		t.Fatalf("password share export leaves: %+v", leaves)
	}
}

// An app token can read its own app but must not reach any management
// endpoint; only a global session may.
func TestAppTokenCannotManage(t *testing.T) {
	h := newHandlerWith(t, Options{
		Store:        newMultiAppStore(),
		Auth:         auth.Config{Enable: true, Password: "pw", Secret: "secret", Expire: time.Hour},
		AppPWs:       map[string]string{"default": "pw1", "b": "pw2"},
		ShareActions: []ShareActionSpec{{Name: "Copy", Action: "copy"}},
		CacheClear:   func() {},
		CacheWarm:    func() {},
		Generator: []GeneratorSpec{{
			Name: "model", Title: "Model",
			Files: []GeneratorFile{{Name: "a", Path: "a", Template: "x"}},
		}},
	})
	admin := login(t, h)
	if !decodeDataOf[bool](t, do(t, h, "POST", "/ui/v1/addApiShare",
		`{"key":"s1","name":"S","type":"all","password":"sp"}`, admin)) {
		t.Fatal("seed share failed")
	}
	appToken := loginApp(t, h, "default|v1", "pw1")

	for _, tc := range []struct{ name, path, body string }{
		{"add share", "/ui/v1/addApiShare", `{"name":"x","type":"all"}`},
		{"share list", "/ui/v1/getApiShareList", `{}`},
		{"share detail", "/ui/v1/getApiShareDetail", `{"key":"s1"}`},
		{"share delete", "/ui/v1/deleteApiShare", `{"key":"s1"}`},
		{"share action", "/ui/v1/handleApiShareAction", `{"key":"s1","index":0}`},
		{"cache clear", "/ui/v1/cache/clear", `{}`},
		{"cache warm", "/ui/v1/cache/warm", `{}`},
		{"generator", "/ui/v1/generator", `{"index":0}`},
		{"export share by key", "/ui/v1/exportAllData", `{"key":"s1"}`},
		{"export all apps", "/ui/v1/exportAllData", `{}`},
	} {
		rec := do(t, h, "POST", tc.path, tc.body, appToken)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("%s with app token: %d %s", tc.name, rec.Code, rec.Body.String())
		}
	}
	// The seeded share is untouched.
	if detail := decodeData(t, do(t, h, "POST", "/ui/v1/getApiShareDetail", `{"key":"s1"}`, admin)); detail["name"] != "S" {
		t.Fatalf("share mutated: %+v", detail)
	}
}

// exportLeafKeys walks an export tree and returns leaf menuKeys in order.
func exportLeafKeys(nodes []any) []string {
	out := []string{}
	var walk func([]any)
	walk = func(nodes []any) {
		for _, node := range nodes {
			m, _ := node.(map[string]any)
			if children, ok := m["children"].([]any); ok {
				walk(children)
				continue
			}
			if key, _ := m["menuKey"].(string); key != "" {
				out = append(out, key)
			}
		}
	}
	walk(nodes)
	return out
}

func findExportLeaf(t *testing.T, nodes []any, menuKey string) map[string]any {
	t.Helper()
	var found map[string]any
	var walk func([]any)
	walk = func(nodes []any) {
		for _, node := range nodes {
			m := node.(map[string]any)
			if children, ok := m["children"].([]any); ok {
				walk(children)
			} else if m["menuKey"] == menuKey {
				found = m
			}
		}
	}
	walk(nodes)
	if found == nil {
		t.Fatalf("export leaf %s not found", menuKey)
	}
	return found
}

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

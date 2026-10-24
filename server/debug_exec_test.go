package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/erikwang2013/apidoc-go/auth"
	"github.com/erikwang2013/apidoc-go/debug"
	"github.com/erikwang2013/apidoc-go/model"
	"github.com/erikwang2013/apidoc-go/store"
)

type captureDebugger struct {
	spec debug.Spec
	got  debug.Request
	resp *debug.Response
	err  error
}

func (d *captureDebugger) Protocol() string { return "mqtt" }
func (d *captureDebugger) Spec() debug.Spec { return d.spec }
func (d *captureDebugger) Exec(ctx context.Context, req debug.Request) (*debug.Response, error) {
	d.got = req
	if d.err != nil {
		return nil, d.err
	}
	if d.resp != nil {
		return d.resp, nil
	}
	return &debug.Response{Status: "PUBACK", Body: []byte("ok"), Encoding: "text"}, nil
}

func newDebugStore() *store.Store {
	s := store.New("p")
	s.Register("PUBLISH", "mqtt://broker/topic", model.Doc{
		App: "a", Version: "v1", Protocol: "mqtt", Title: "publish",
		Params: []model.Param{
			{Name: "qos", In: "option", Type: "int"},
			{Name: "payload", In: "payload", Type: "string"},
		},
	})
	return s
}

func debugPost(t *testing.T, h http.Handler, body string, csrf bool) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(http.MethodPost, "/api/debug/exec", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	if csrf {
		r.Header.Set("X-Apidoc-Debug", "1")
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	return rec
}

func TestDebugExecSuccess(t *testing.T) {
	d := &captureDebugger{spec: debug.Spec{
		Mode: debug.ModeServer, Actions: []string{"PUBLISH"},
		Encodings: []string{"json", "text"},
	}}
	h := Handler(Opts{Prefix: "/apidoc", Store: newDebugStore(),
		Debuggers: map[string]debug.Debugger{"mqtt": d}, DebugAllowUnauth: true})
	id := store.ActionID("a", "v1", "PUBLISH", "mqtt://broker/topic")
	body := fmt.Sprintf(`{"id":%q,"method":"PUBLISH","url":"mqtt://broker/topic",`+
		`"params":[{"name":"qos","in":"option","value":"1"},`+
		`{"name":"payload","in":"payload","value":"hello"}],`+
		`"body":"aGVsbG8=","encoding":"text",`+
		`"headers":{"X-A":"b"},"options":{"clientId":"c"}}`, id)
	rec := debugPost(t, h, body, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var out struct {
		Data struct {
			Status     string            `json:"status"`
			Body       []byte            `json:"body"`
			Encoding   string            `json:"encoding"`
			DurationMS int64             `json:"duration_ms"`
			Headers    map[string]string `json:"headers"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Data.Status != "PUBACK" || string(out.Data.Body) != "ok" ||
		out.Data.Encoding != "text" || out.Data.DurationMS < 0 {
		t.Fatalf("response data wrong: %+v", out.Data)
	}
	got := d.got
	if got.ActionID != id || got.Method != "PUBLISH" || got.URL != "mqtt://broker/topic" ||
		got.Encoding != "text" || string(got.Body) != "hello" ||
		got.Headers["X-A"] != "b" || got.Options["clientId"] != "c" ||
		len(got.Params) != 2 || got.Params[1].Value != "hello" {
		t.Fatalf("adapter request wrong: %+v", got)
	}
}

func TestDebugExecErrors(t *testing.T) {
	id := store.ActionID("a", "v1", "PUBLISH", "mqtt://broker/topic")
	valid := func(method, url string) string {
		return fmt.Sprintf(`{"id":%q,"method":%q,"url":%q,`+
			`"params":[{"name":"qos","in":"option","value":"1"}],`+
			`"body":"","encoding":"json"}`, id, method, url)
	}
	spec := debug.Spec{Mode: debug.ModeServer, Actions: []string{"PUBLISH"}}
	newHandler := func(d debug.Debugger, allow bool) http.Handler {
		return Handler(Opts{Prefix: "/apidoc", Store: newDebugStore(),
			Debuggers: map[string]debug.Debugger{"mqtt": d}, DebugAllowUnauth: allow})
	}
	tests := []struct {
		name   string
		h      http.Handler
		body   string
		csrf   bool
		status int
	}{
		{"missing csrf", newHandler(&captureDebugger{spec: spec}, true), valid("PUBLISH", "mqtt://broker/topic"), false, http.StatusForbidden},
		{"unknown id", newHandler(&captureDebugger{spec: spec}, true), `{"id":"nope"}`, true, http.StatusNotFound},
		{"no debugger", Handler(Opts{Prefix: "/apidoc", Store: newDebugStore(), DebugAllowUnauth: true}), valid("PUBLISH", "mqtt://broker/topic"), true, http.StatusBadRequest},
		{"browser mode", newHandler(&captureDebugger{spec: debug.HTTPBrowserSpec()}, true), valid("PUBLISH", "mqtt://broker/topic"), true, http.StatusBadRequest},
		{"action not allowed", newHandler(&captureDebugger{spec: spec}, true), valid("DELETE", "mqtt://broker/topic"), true, http.StatusBadRequest},
		{"target override denied", newHandler(&captureDebugger{spec: spec}, true), valid("PUBLISH", "mqtt://evil/topic"), true, http.StatusBadRequest},
		{"debug disabled", newHandler(&captureDebugger{spec: spec}, false), valid("PUBLISH", "mqtt://broker/topic"), true, http.StatusForbidden},
		{"adapter error", newHandler(&captureDebugger{spec: spec, err: fmt.Errorf("boom")}, true), valid("PUBLISH", "mqtt://broker/topic"), true, http.StatusBadGateway},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := debugPost(t, tc.h, tc.body, tc.csrf)
			if rec.Code != tc.status {
				t.Fatalf("status = %d, want %d: %s", rec.Code, tc.status, rec.Body.String())
			}
		})
	}
}

func TestDebugExecTargetOverrideAllowed(t *testing.T) {
	d := &captureDebugger{spec: debug.Spec{
		Mode: debug.ModeServer, Actions: []string{"PUBLISH"}, AllowTargetOverride: true,
	}}
	h := Handler(Opts{Prefix: "/apidoc", Store: newDebugStore(),
		Debuggers: map[string]debug.Debugger{"mqtt": d}, DebugAllowUnauth: true})
	id := store.ActionID("a", "v1", "PUBLISH", "mqtt://broker/topic")
	body := fmt.Sprintf(`{"id":%q,"method":"PUBLISH","url":"mqtt://other/topic",`+
		`"params":[],"body":"","encoding":"json"}`, id)
	if rec := debugPost(t, h, body, true); rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if d.got.URL != "mqtt://other/topic" {
		t.Fatalf("override not passed: %+v", d.got)
	}
}

func TestDebugExecAuth(t *testing.T) {
	id := store.ActionID("a", "v1", "PUBLISH", "mqtt://broker/topic")
	body := fmt.Sprintf(`{"id":%q,"method":"PUBLISH","url":"mqtt://broker/topic",`+
		`"params":[],"body":"","encoding":"json"}`, id)
	d := &captureDebugger{spec: debug.Spec{Mode: debug.ModeServer, Actions: []string{"PUBLISH"}}}

	// Auth enabled: no session cookie -> 401 from withAuth.
	h := Handler(Opts{Prefix: "/apidoc", Store: newDebugStore(),
		Debuggers:        map[string]debug.Debugger{"mqtt": d},
		Auth:             auth.Config{Enable: true, Password: "pw", Secret: "sec"},
		DebugAllowUnauth: true})
	if rec := debugPost(t, h, body, true); rec.Code != http.StatusUnauthorized {
		t.Fatalf("auth on without cookie: %d", rec.Code)
	}

	// App-protected action with auth off requires the app token.
	h = Handler(Opts{Prefix: "/apidoc", Store: newDebugStore(),
		Debuggers:        map[string]debug.Debugger{"mqtt": d},
		AppPWs:           map[string]string{"a": "apppw"},
		Auth:             auth.Config{Secret: "sec"},
		DebugAllowUnauth: true})
	if rec := debugPost(t, h, body, true); rec.Code != http.StatusUnauthorized {
		t.Fatalf("app token missing: %d", rec.Code)
	}
	tok, err := auth.Issue("sec", time.Hour, "a")
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodPost, "/api/debug/exec", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("X-Apidoc-Debug", "1")
	r.Header.Set("X-Apidoc-App-Token", tok)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	if rec.Code != http.StatusOK {
		t.Fatalf("app token valid: %d %s", rec.Code, rec.Body.String())
	}
}

type blockingDebugger struct {
	started chan struct{}
	release chan struct{}
}

func (d *blockingDebugger) Protocol() string { return "mqtt" }
func (d *blockingDebugger) Spec() debug.Spec {
	return debug.Spec{Mode: debug.ModeServer, Actions: []string{"PUBLISH"}}
}
func (d *blockingDebugger) Exec(ctx context.Context, req debug.Request) (*debug.Response, error) {
	close(d.started)
	select {
	case <-d.release:
		return &debug.Response{Status: "OK"}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func TestDebugExecTimeout(t *testing.T) {
	d := &blockingDebugger{started: make(chan struct{}), release: make(chan struct{})}
	h := Handler(Opts{Prefix: "/apidoc", Store: newDebugStore(),
		Debuggers:        map[string]debug.Debugger{"mqtt": d},
		DebugAllowUnauth: true, DebugTimeout: 20 * time.Millisecond})
	id := store.ActionID("a", "v1", "PUBLISH", "mqtt://broker/topic")
	body := fmt.Sprintf(`{"id":%q,"method":"PUBLISH","url":"mqtt://broker/topic",`+
		`"params":[],"body":"","encoding":"json"}`, id)
	rec := debugPost(t, h, body, true)
	if rec.Code != http.StatusGatewayTimeout {
		t.Fatalf("timeout status = %d: %s", rec.Code, rec.Body.String())
	}
}

func TestDebugExecConcurrency(t *testing.T) {
	d := &blockingDebugger{started: make(chan struct{}), release: make(chan struct{})}
	h := Handler(Opts{Prefix: "/apidoc", Store: newDebugStore(),
		Debuggers:        map[string]debug.Debugger{"mqtt": d},
		DebugAllowUnauth: true, DebugMaxConcurrent: 1})
	id := store.ActionID("a", "v1", "PUBLISH", "mqtt://broker/topic")
	body := fmt.Sprintf(`{"id":%q,"method":"PUBLISH","url":"mqtt://broker/topic",`+
		`"params":[],"body":"","encoding":"json"}`, id)

	first := make(chan int, 1)
	go func() { first <- debugPost(t, h, body, true).Code }()
	<-d.started
	if rec := debugPost(t, h, body, true); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("second status = %d", rec.Code)
	}
	close(d.release)
	if code := <-first; code != http.StatusOK {
		t.Fatalf("first status = %d", code)
	}
}

func TestDebugExecCaps(t *testing.T) {
	spec := debug.Spec{Mode: debug.ModeServer, Actions: []string{"PUBLISH"}}
	large := &captureDebugger{spec: spec, resp: &debug.Response{
		Status: "OK", Body: make([]byte, maxDebugResponse+1),
	}}
	h := Handler(Opts{Prefix: "/apidoc", Store: newDebugStore(),
		Debuggers: map[string]debug.Debugger{"mqtt": large}, DebugAllowUnauth: true})
	id := store.ActionID("a", "v1", "PUBLISH", "mqtt://broker/topic")
	body := fmt.Sprintf(`{"id":%q,"method":"PUBLISH","url":"mqtt://broker/topic",`+
		`"params":[],"body":"","encoding":"json"}`, id)
	rec := debugPost(t, h, body, true)
	var out struct {
		Data struct {
			Body      []byte `json:"body"`
			Truncated bool   `json:"truncated"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if !out.Data.Truncated || len(out.Data.Body) != maxDebugResponse {
		t.Fatalf("response not capped: truncated=%v len=%d", out.Data.Truncated, len(out.Data.Body))
	}

	tooBig := fmt.Sprintf(`{"id":%q,"params":[{"name":"qos","in":"option","value":%q}]}`,
		id, strings.Repeat("x", maxDebugBody))
	if rec := debugPost(t, h, tooBig, true); rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("body cap status = %d", rec.Code)
	}
}

func TestDebugExecParamWithoutIn(t *testing.T) {
	s := store.New("p")
	s.Register("SEND", "tcp://127.0.0.1:19000", model.Doc{
		App: "a", Version: "v1", Protocol: "tcp", Title: "send",
		Params: []model.Param{{Name: "payload", Type: "text"}},
	})
	d := &captureDebugger{spec: debug.Spec{Mode: debug.ModeServer, Actions: []string{"SEND"}}}
	h := Handler(Opts{Prefix: "/apidoc", Store: s,
		Debuggers: map[string]debug.Debugger{"tcp": d}, DebugAllowUnauth: true})
	id := store.ActionID("a", "v1", "SEND", "tcp://127.0.0.1:19000")
	body := fmt.Sprintf(`{"id":%q,"method":"SEND","url":"tcp://127.0.0.1:19000",`+
		`"params":[{"name":"payload","in":"","value":"x"}],"body":"","encoding":"json"}`, id)
	if rec := debugPost(t, h, body, true); rec.Code != http.StatusOK {
		t.Fatalf("In-empty param rejected: %d %s", rec.Code, rec.Body.String())
	}
}

func TestDebugExecResponseCaps(t *testing.T) {
	spec := debug.Spec{Mode: debug.ModeServer, Actions: []string{"PUBLISH"}}
	headers := map[string]string{}
	for i := 0; i < maxDebugHeaders+10; i++ {
		headers[fmt.Sprintf("H-%d", i)] = strings.Repeat("v", maxDebugHeaderValue+10)
	}
	d := &captureDebugger{spec: spec, resp: &debug.Response{
		Status:  strings.Repeat("s", maxDebugStatus+10),
		Headers: headers,
		Events:  []debug.Event{{Direction: "in", Data: strings.Repeat("e", maxDebugEventData+10)}},
	}}
	h := Handler(Opts{Prefix: "/apidoc", Store: newDebugStore(),
		Debuggers: map[string]debug.Debugger{"mqtt": d}, DebugAllowUnauth: true})
	id := store.ActionID("a", "v1", "PUBLISH", "mqtt://broker/topic")
	body := fmt.Sprintf(`{"id":%q,"method":"PUBLISH","url":"mqtt://broker/topic",`+
		`"params":[],"body":"","encoding":"json"}`, id)
	rec := debugPost(t, h, body, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var out struct {
		Data debugExecResp `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	r := out.Data
	if !r.Truncated {
		t.Fatal("truncation not reported")
	}
	if len(r.Status) > maxDebugStatus {
		t.Fatalf("status not capped: %d", len(r.Status))
	}
	if len(r.Events) != 1 || len(r.Events[0].Data) > maxDebugEventData {
		t.Fatalf("event data not capped: %+v", r.Events)
	}
	if len(r.Headers) > maxDebugHeaders {
		t.Fatalf("headers not capped: %d", len(r.Headers))
	}
	for k, v := range r.Headers {
		if len(k) > maxDebugHeaderValue || len(v) > maxDebugHeaderValue {
			t.Fatalf("header value not capped: %q (%d)", k, len(v))
		}
	}
}

func TestDebugContentTypeStrict(t *testing.T) {
	d := &captureDebugger{spec: debug.Spec{Mode: debug.ModeServer, Actions: []string{"PUBLISH"}}}
	h := Handler(Opts{Prefix: "/apidoc", Store: newDebugStore(),
		Debuggers: map[string]debug.Debugger{"mqtt": d}, DebugAllowUnauth: true})
	id := store.ActionID("a", "v1", "PUBLISH", "mqtt://broker/topic")
	body := fmt.Sprintf(`{"id":%q,"method":"PUBLISH","url":"mqtt://broker/topic",`+
		`"params":[],"body":"","encoding":"json"}`, id)
	cases := []struct {
		ct     string
		status int
	}{
		{"application/json", http.StatusOK},
		{"application/json; charset=utf-8", http.StatusOK},
		{"application/jsonfoo", http.StatusUnsupportedMediaType},
		{"text/json", http.StatusUnsupportedMediaType},
		{"", http.StatusUnsupportedMediaType},
	}
	for _, tc := range cases {
		r := httptest.NewRequest(http.MethodPost, "/api/debug/exec", strings.NewReader(body))
		if tc.ct != "" {
			r.Header.Set("Content-Type", tc.ct)
		}
		r.Header.Set("X-Apidoc-Debug", "1")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, r)
		if rec.Code != tc.status {
			t.Errorf("content-type %q status = %d, want %d", tc.ct, rec.Code, tc.status)
		}
	}
}

func TestDebugExecInputCaps(t *testing.T) {
	h := Handler(Opts{Prefix: "/apidoc", Store: newDebugStore(),
		Debuggers: map[string]debug.Debugger{"mqtt": &captureDebugger{
			spec: debug.Spec{Mode: debug.ModeServer, Actions: []string{"PUBLISH"}},
		}}, DebugAllowUnauth: true})
	id := store.ActionID("a", "v1", "PUBLISH", "mqtt://broker/topic")
	params := make([]debug.Value, maxDebugParams+1)
	for i := range params {
		params[i] = debug.Value{Name: "qos", In: "option", Value: "1"}
	}
	b, _ := json.Marshal(debugExecReq{ID: id, Method: "PUBLISH",
		URL: "mqtt://broker/topic", Params: params})
	if rec := debugPost(t, h, string(b), true); rec.Code != http.StatusBadRequest {
		t.Fatalf("param count cap status = %d", rec.Code)
	}
	long := strings.Repeat("x", maxDebugParamValue+1)
	b, _ = json.Marshal(debugExecReq{ID: id, Method: "PUBLISH",
		URL:    "mqtt://broker/topic",
		Params: []debug.Value{{Name: "qos", In: "option", Value: long}}})
	if rec := debugPost(t, h, string(b), true); rec.Code != http.StatusBadRequest {
		t.Fatalf("param value cap status = %d", rec.Code)
	}
}

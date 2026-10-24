package server

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/erikwang2013/apidoc-go/auth"
	"github.com/erikwang2013/apidoc-go/debug"
)

func TestDebugOpenSuccess(t *testing.T) {
	d := &fakeStreamDebugger{spec: streamSpec(), sess: newFakeStreamSession()}
	h := streamHandler(d, Opts{DebugAllowUnauth: true})
	rec := postDebug(t, h, "/api/debug/open", streamOpenBody(t), true)
	if rec.Code != http.StatusOK {
		t.Fatalf("open status %d: %s", rec.Code, rec.Body.String())
	}
	if sessionIDFrom(t, rec) == "" {
		t.Fatal("empty session id")
	}
	got := d.request()
	if got.ActionID != streamActionID() || got.Method != "CONNECT" ||
		got.URL != "ws://stream/topic" || got.Encoding != "text" ||
		string(got.Body) != "hello" || len(got.Params) != 1 ||
		got.Params[0].Value != "news" {
		t.Fatalf("adapter request wrong: %+v", got)
	}
}

func streamHandler(d debug.Debugger, o Opts) http.Handler {
	o.Store = newStreamStore()
	o.Debuggers = map[string]debug.Debugger{"ws": d}
	return Handler(o)
}

func TestDebugOpenRequiresStreamDebugger(t *testing.T) {
	plain := &captureDebugger{spec: debug.Spec{
		Mode: debug.ModeServer, Actions: []string{"CONNECT"},
		Encodings: []string{"text", "json"},
	}}
	h := streamHandler(plain, Opts{DebugAllowUnauth: true})
	rec := postDebug(t, h, "/api/debug/open", streamOpenBody(t), true)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("non-streaming open status %d: %s", rec.Code, rec.Body.String())
	}
}

func TestDebugOpenGuards(t *testing.T) {
	d := &fakeStreamDebugger{spec: streamSpec(), sess: newFakeStreamSession()}
	allow := Opts{DebugAllowUnauth: true}
	tests := []struct {
		name   string
		h      http.Handler
		body   string
		csrf   bool
		ctype  string
		status int
	}{
		{"missing csrf", streamHandler(d, allow), streamOpenBody(t), false, "application/json", http.StatusForbidden},
		{"content-type prefix collision", streamHandler(d, allow), streamOpenBody(t), true, "application/jsonfoo", http.StatusUnsupportedMediaType},
		{"missing content-type", streamHandler(d, allow), streamOpenBody(t), true, "", http.StatusUnsupportedMediaType},
		{"unknown action", streamHandler(d, allow), `{"id":"nope"}`, true, "application/json", http.StatusNotFound},
		{"debug disabled", streamHandler(d, Opts{}), streamOpenBody(t), true, "application/json", http.StatusForbidden},
		{"open error", streamHandler(&fakeStreamDebugger{spec: streamSpec(), openErr: fmt.Errorf("boom")}, allow), streamOpenBody(t), true, "application/json", http.StatusBadGateway},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "/api/debug/open", strings.NewReader(tc.body))
			if tc.ctype != "" {
				r.Header.Set("Content-Type", tc.ctype)
			} else {
				r.Header["Content-Type"] = nil
			}
			if tc.csrf {
				r.Header.Set("X-Apidoc-Debug", "1")
			}
			rec := httptest.NewRecorder()
			tc.h.ServeHTTP(rec, r)
			if rec.Code != tc.status {
				t.Fatalf("status = %d, want %d: %s", rec.Code, tc.status, rec.Body.String())
			}
		})
	}
}

func TestDebugOpenBindings(t *testing.T) {
	base := func() Opts { return Opts{DebugAllowUnauth: true} }
	d := &fakeStreamDebugger{spec: streamSpec(), sess: newFakeStreamSession()}

	// Method outside spec.Actions.
	o := base()
	h := streamHandler(d, o)
	body := fmt.Sprintf(`{"id":%q,"method":"DELETE","url":"ws://stream/topic","encoding":"text"}`, streamActionID())
	if rec := postDebug(t, h, "/api/debug/open", body, true); rec.Code != http.StatusBadRequest {
		t.Fatalf("disallowed method status %d", rec.Code)
	}

	// Target override without permission.
	body = fmt.Sprintf(`{"id":%q,"method":"CONNECT","url":"ws://evil/topic","encoding":"text"}`, streamActionID())
	if rec := postDebug(t, h, "/api/debug/open", body, true); rec.Code != http.StatusBadRequest {
		t.Fatalf("target override status %d", rec.Code)
	}

	// Unknown param.
	body = fmt.Sprintf(`{"id":%q,"method":"CONNECT","url":"ws://stream/topic",`+
		`"params":[{"name":"other","in":"option","value":"x"}],"encoding":"text"}`, streamActionID())
	if rec := postDebug(t, h, "/api/debug/open", body, true); rec.Code != http.StatusBadRequest {
		t.Fatalf("unknown param status %d", rec.Code)
	}

	// Unsupported encoding.
	body = fmt.Sprintf(`{"id":%q,"method":"CONNECT","url":"ws://stream/topic","encoding":"xml"}`, streamActionID())
	if rec := postDebug(t, h, "/api/debug/open", body, true); rec.Code != http.StatusBadRequest {
		t.Fatalf("unsupported encoding status %d", rec.Code)
	}
}

func TestDebugStreamSendClose(t *testing.T) {
	fs := newFakeStreamSession()
	d := &fakeStreamDebugger{spec: streamSpec(), sess: fs}
	h := streamHandler(d, Opts{DebugAllowUnauth: true})
	open := postDebug(t, h, "/api/debug/open", streamOpenBody(t), true)
	if open.Code != http.StatusOK {
		t.Fatalf("open status %d: %s", open.Code, open.Body.String())
	}
	sid := sessionIDFrom(t, open)

	send := fmt.Sprintf(`{"session_id":%q,"body":"d29ybGQ=","encoding":"text"}`, sid)
	if rec := postDebug(t, h, "/api/debug/send", send, true); rec.Code != http.StatusOK {
		t.Fatalf("send status %d: %s", rec.Code, rec.Body.String())
	}
	sent, metas := fs.sentMessages()
	if len(sent) != 1 || string(sent[0]) != "world" || metas[0]["encoding"] != "text" {
		t.Fatalf("session send wrong: %q %+v", sent, metas)
	}

	closeBody := fmt.Sprintf(`{"session_id":%q}`, sid)
	if rec := postDebug(t, h, "/api/debug/close", closeBody, true); rec.Code != http.StatusOK {
		t.Fatalf("close status %d: %s", rec.Code, rec.Body.String())
	}
	select {
	case <-fs.closed:
	default:
		t.Fatal("adapter session not closed")
	}
	if rec := postDebug(t, h, "/api/debug/close", closeBody, true); rec.Code != http.StatusNotFound {
		t.Fatalf("second close status %d", rec.Code)
	}
}

func TestDebugStreamSendGuards(t *testing.T) {
	fs := newFakeStreamSession()
	d := &fakeStreamDebugger{spec: streamSpec(), sess: fs}
	h := streamHandler(d, Opts{DebugAllowUnauth: true})
	open := postDebug(t, h, "/api/debug/open", streamOpenBody(t), true)
	sid := sessionIDFrom(t, open)

	if rec := postDebug(t, h, "/api/debug/send",
		fmt.Sprintf(`{"session_id":%q,"body":""}`, sid), false); rec.Code != http.StatusForbidden {
		t.Fatalf("send without csrf status %d", rec.Code)
	}
	if rec := postDebug(t, h, "/api/debug/send", `{"session_id":"nope"}`, true); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown session status %d", rec.Code)
	}
	huge := fmt.Sprintf(`{"session_id":%q,"body":%q}`, sid, strings.Repeat("A", maxDebugBody+10))
	if rec := postDebug(t, h, "/api/debug/send", huge, true); rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized message status %d", rec.Code)
	}
}

func TestDebugStreamAuthBinding(t *testing.T) {
	const secret = "sec"
	fs := newFakeStreamSession()
	d := &fakeStreamDebugger{spec: streamSpec(), sess: fs}
	h := streamHandler(d, Opts{
		DebugAllowUnauth: true,
		Auth:             auth.Config{Secret: secret},
		AppPWs:           map[string]string{"a": "pw"},
	})

	// No app token.
	if rec := postDebug(t, h, "/api/debug/open", streamOpenBody(t), true); rec.Code != http.StatusUnauthorized {
		t.Fatalf("open without token status %d", rec.Code)
	}
	// Token for another app.
	wrong, err := auth.Issue(secret, time.Hour, "b")
	if err != nil {
		t.Fatal(err)
	}
	if rec := postDebugToken(t, h, "/api/debug/open", streamOpenBody(t), wrong); rec.Code != http.StatusUnauthorized {
		t.Fatalf("open with foreign token status %d", rec.Code)
	}
	tok, err := auth.Issue(secret, time.Hour, "a")
	if err != nil {
		t.Fatal(err)
	}
	open := postDebugToken(t, h, "/api/debug/open", streamOpenBody(t), tok)
	if open.Code != http.StatusOK {
		t.Fatalf("open with token status %d: %s", open.Code, open.Body.String())
	}
	sid := sessionIDFrom(t, open)

	// Send and close must re-check the bound app.
	send := fmt.Sprintf(`{"session_id":%q,"body":"eA=="}`, sid)
	if rec := postDebug(t, h, "/api/debug/send", send, true); rec.Code != http.StatusUnauthorized {
		t.Fatalf("send without token status %d", rec.Code)
	}
	if rec := postDebugToken(t, h, "/api/debug/send", send, tok); rec.Code != http.StatusOK {
		t.Fatalf("send with token status %d: %s", rec.Code, rec.Body.String())
	}
	closeBody := fmt.Sprintf(`{"session_id":%q}`, sid)
	if rec := postDebugToken(t, h, "/api/debug/close", closeBody, tok); rec.Code != http.StatusOK {
		t.Fatalf("close with token status %d", rec.Code)
	}
}

func postDebugToken(t *testing.T, h http.Handler, path, body, token string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("X-Apidoc-Debug", "1")
	r.Header.Set("X-Apidoc-App-Token", token)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	return rec
}

func TestDebugEventsSSE(t *testing.T) {
	old := sseHeartbeat
	sseHeartbeat = 20 * time.Millisecond
	defer func() { sseHeartbeat = old }()

	fs := newFakeStreamSession()
	d := &fakeStreamDebugger{spec: streamSpec(), sess: fs}
	h := streamHandler(d, Opts{DebugAllowUnauth: true})
	ts := httptest.NewServer(h)
	defer ts.Close()

	open := postDebug(t, h, "/api/debug/open", streamOpenBody(t), true)
	if open.Code != http.StatusOK {
		t.Fatalf("open status %d: %s", open.Code, open.Body.String())
	}
	sid := sessionIDFrom(t, open)

	resp, err := http.Get(ts.URL + "/api/debug/events?session_id=" + url.QueryEscape(sid))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("events status %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("events content-type %q", ct)
	}
	fs.events <- debug.Event{Time: time.Now(), Direction: "in", Name: "msg", Data: "hello"}

	lines := make(chan string, 32)
	go func() {
		sc := bufio.NewScanner(resp.Body)
		for sc.Scan() {
			lines <- sc.Text()
		}
		close(lines)
	}()
	var gotData, gotPing bool
	timeout := time.After(3 * time.Second)
	for !gotData || !gotPing {
		select {
		case line, ok := <-lines:
			if !ok {
				t.Fatal("SSE stream ended early")
			}
			if strings.HasPrefix(line, ": ping") {
				gotPing = true
			}
			if strings.HasPrefix(line, "data: ") {
				var ev debug.Event
				if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &ev); err != nil {
					t.Fatalf("bad SSE event %q: %v", line, err)
				}
				if ev.Data != "hello" || ev.Direction != "in" || ev.Name != "msg" {
					t.Fatalf("event = %+v", ev)
				}
				gotData = true
			}
		case <-timeout:
			t.Fatalf("timed out waiting for SSE data/ping: data=%v ping=%v", gotData, gotPing)
		}
	}
	// Client disconnect must close the adapter session.
	resp.Body.Close()
	eventually(t, 2*time.Second, func() bool {
		select {
		case <-fs.closed:
			return true
		default:
			return false
		}
	})
}

func TestDebugEventsSingleReaderAndAuth(t *testing.T) {
	fs := newFakeStreamSession()
	d := &fakeStreamDebugger{spec: streamSpec(), sess: fs}
	h := streamHandler(d, Opts{DebugAllowUnauth: true})
	ts := httptest.NewServer(h)
	defer ts.Close()

	open := postDebug(t, h, "/api/debug/open", streamOpenBody(t), true)
	sid := sessionIDFrom(t, open)

	first, err := http.Get(ts.URL + "/api/debug/events?session_id=" + url.QueryEscape(sid))
	if err != nil {
		t.Fatal(err)
	}
	if first.StatusCode != http.StatusOK {
		t.Fatalf("first events status %d", first.StatusCode)
	}
	second, err := http.Get(ts.URL + "/api/debug/events?session_id=" + url.QueryEscape(sid))
	if err != nil {
		t.Fatal(err)
	}
	if second.StatusCode != http.StatusConflict {
		t.Fatalf("second reader status %d", second.StatusCode)
	}
	second.Body.Close()
	first.Body.Close()

	// Unknown session.
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/debug/events?session_id=nope", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown session status %d", rec.Code)
	}
}

func TestDebugEventsAppTokenQuery(t *testing.T) {
	const secret = "sec"
	fs := newFakeStreamSession()
	d := &fakeStreamDebugger{spec: streamSpec(), sess: fs}
	h := streamHandler(d, Opts{
		DebugAllowUnauth: true,
		Auth:             auth.Config{Secret: secret},
		AppPWs:           map[string]string{"a": "pw"},
	})
	tok, err := auth.Issue(secret, time.Hour, "a")
	if err != nil {
		t.Fatal(err)
	}
	open := postDebugToken(t, h, "/api/debug/open", streamOpenBody(t), tok)
	if open.Code != http.StatusOK {
		t.Fatalf("open status %d: %s", open.Code, open.Body.String())
	}
	sid := sessionIDFrom(t, open)

	// Without the app token (EventSource cannot send headers).
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/debug/events?session_id="+url.QueryEscape(sid), nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("events without token status %d", rec.Code)
	}

	ts := httptest.NewServer(h)
	defer ts.Close()
	resp, err := http.Get(ts.URL + "/api/debug/events?session_id=" + url.QueryEscape(sid) +
		"&app_token=" + url.QueryEscape(tok))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("events with query token status %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestDebugEventsCapsData(t *testing.T) {
	fs := newFakeStreamSession()
	d := &fakeStreamDebugger{spec: streamSpec(), sess: fs}
	h := streamHandler(d, Opts{DebugAllowUnauth: true})
	ts := httptest.NewServer(h)
	defer ts.Close()

	open := postDebug(t, h, "/api/debug/open", streamOpenBody(t), true)
	sid := sessionIDFrom(t, open)
	resp, err := http.Get(ts.URL + "/api/debug/events?session_id=" + url.QueryEscape(sid))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	fs.events <- debug.Event{Direction: "in", Data: strings.Repeat("x", maxDebugEventData+50)}
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 128<<10), 1<<20)
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var ev debug.Event
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &ev); err != nil {
			t.Fatal(err)
		}
		if len(ev.Data) > maxDebugEventData {
			t.Fatalf("event data not capped: %d", len(ev.Data))
		}
		if !strings.Contains(ev.Name, "truncated") {
			t.Fatalf("truncation not marked: %q", ev.Name)
		}
		return
	}
	t.Fatal("no event received")
}

func TestStreamingUIAssets(t *testing.T) {
	js := string(debugJSPayload)
	for _, want := range []string{
		"connectStream", "sendStreamMessage", "closeStream", "EventSource",
		"/api/debug/open", "/api/debug/send", "/api/debug/close", "/api/debug/events",
		"dg-transcript", "dataset.required", "truthy",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("debug.js missing %q", want)
		}
	}
	if strings.Contains(js, "innerHTML") {
		t.Fatal("debug.js must not use innerHTML")
	}
	// Both the exec and streaming send paths surface network failures.
	if n := strings.Count(js, "request failed"); n < 2 {
		t.Fatalf("network error handling missing: %d", n)
	}
	page := string(indexHTML)
	for _, want := range []string{"dg-transcript", "connectStream()", "sendStreamMessage()", "closeStream()"} {
		if !strings.Contains(page, want) {
			t.Errorf("index.html missing %q", want)
		}
	}
}

type blockingOpenDebugger struct {
	fakeStreamDebugger
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (d *blockingOpenDebugger) Open(ctx context.Context, req debug.Request) (debug.Session, error) {
	d.once.Do(func() { close(d.started) })
	select {
	case <-d.release:
		return newFakeStreamSession(), nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func TestDebugOpenConcurrencyCap(t *testing.T) {
	d := &blockingOpenDebugger{
		fakeStreamDebugger: fakeStreamDebugger{spec: streamSpec()},
		started:            make(chan struct{}),
		release:            make(chan struct{}),
	}
	h := streamHandler(d, Opts{DebugAllowUnauth: true, DebugMaxConcurrent: 1})
	first := make(chan int, 1)
	go func() { first <- postDebug(t, h, "/api/debug/open", streamOpenBody(t), true).Code }()
	<-d.started

	second := make(chan int, 1)
	go func() { second <- postDebug(t, h, "/api/debug/open", streamOpenBody(t), true).Code }()
	select {
	case code := <-second:
		if code != http.StatusTooManyRequests {
			t.Fatalf("second open status = %d, want 429", code)
		}
	case <-time.After(500 * time.Millisecond):
		close(d.release)
		<-first
		t.Fatal("second open was not capped")
	}
	close(d.release)
	if code := <-first; code != http.StatusOK {
		t.Fatalf("first open status = %d", code)
	}
}

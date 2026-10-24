package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/erikwang2013/apidoc-go/debug"
	"github.com/erikwang2013/apidoc-go/model"
	"github.com/erikwang2013/apidoc-go/store"
)

type fakeStreamSession struct {
	mu      sync.Mutex
	events  chan debug.Event
	sent    [][]byte
	metas   []map[string]string
	closed  chan struct{}
	sendErr error
}

func newFakeStreamSession() *fakeStreamSession {
	return &fakeStreamSession{events: make(chan debug.Event, 4), closed: make(chan struct{})}
}

func (s *fakeStreamSession) Send(ctx context.Context, msg []byte, meta map[string]string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sendErr != nil {
		return s.sendErr
	}
	s.sent = append(s.sent, append([]byte(nil), msg...))
	s.metas = append(s.metas, meta)
	return nil
}

func (s *fakeStreamSession) Events() <-chan debug.Event { return s.events }

func (s *fakeStreamSession) Close() error {
	select {
	case <-s.closed:
	default:
		close(s.closed)
	}
	return nil
}

func (s *fakeStreamSession) sentMessages() ([][]byte, []map[string]string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sent, s.metas
}

type fakeStreamDebugger struct {
	mu      sync.Mutex
	spec    debug.Spec
	sess    *fakeStreamSession
	openErr error
	got     debug.Request
	opens   int
}

func (d *fakeStreamDebugger) Protocol() string { return "ws" }
func (d *fakeStreamDebugger) Spec() debug.Spec { return d.spec }
func (d *fakeStreamDebugger) Exec(context.Context, debug.Request) (*debug.Response, error) {
	return nil, fmt.Errorf("not a one-shot debugger")
}
func (d *fakeStreamDebugger) Open(ctx context.Context, req debug.Request) (debug.Session, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.opens++
	d.got = req
	if d.openErr != nil {
		return nil, d.openErr
	}
	return d.sess, nil
}

func (d *fakeStreamDebugger) openCount() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.opens
}

func (d *fakeStreamDebugger) request() debug.Request {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.got
}

func streamSpec() debug.Spec {
	return debug.Spec{
		Mode: debug.ModeServer, Streaming: true,
		Actions: []string{"CONNECT"}, Encodings: []string{"text", "json"},
	}
}

func newStreamStore() *store.Store {
	s := store.New("p")
	s.Register("CONNECT", "ws://stream/topic", model.Doc{
		App: "a", Version: "v1", Protocol: "ws", Title: "stream",
		Params: []model.Param{{Name: "channel", In: "option", Type: "string"}},
	})
	return s
}

func streamActionID() string {
	return store.ActionID("a", "v1", "CONNECT", "ws://stream/topic")
}

func streamOpenBody(t *testing.T) string {
	t.Helper()
	return fmt.Sprintf(`{"id":%q,"method":"CONNECT","url":"ws://stream/topic",`+
		`"params":[{"name":"channel","in":"option","value":"news"}],`+
		`"body":"aGVsbG8=","encoding":"text"}`, streamActionID())
}

// postDebug posts to any /api/debug endpoint with the JSON headers.
func postDebug(t *testing.T, h http.Handler, path, body string, csrf bool) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	if csrf {
		r.Header.Set("X-Apidoc-Debug", "1")
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	return rec
}

func sessionIDFrom(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var out struct {
		Data struct {
			SessionID string `json:"session_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode open response: %v (%s)", err, rec.Body.String())
	}
	if out.Data.SessionID == "" {
		t.Fatalf("no session id: %s", rec.Body.String())
	}
	return out.Data.SessionID
}

func eventually(t *testing.T, timeout time.Duration, f func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if f() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("condition not met before timeout")
}

func TestSessionManagerRandomIDsAndLimit(t *testing.T) {
	m := newSessionManager(2, time.Minute, time.Minute)
	d := &fakeStreamDebugger{sess: newFakeStreamSession()}
	s1, err := m.open(context.Background(), "a", d, debug.Request{})
	if err != nil {
		t.Fatal(err)
	}
	s2, err := m.open(context.Background(), "a", d, debug.Request{})
	if err != nil {
		t.Fatal(err)
	}
	if s1.id == s2.id || len(s1.id) != 43 {
		t.Fatalf("ids not random base64: %q %q", s1.id, s2.id)
	}
	if _, err := m.open(context.Background(), "a", d, debug.Request{}); err == nil {
		t.Fatal("third session over the limit was accepted")
	}
	m.remove(s1.id)
	m.remove(s2.id)
}

func TestSessionManagerIdleTimeout(t *testing.T) {
	m := newSessionManager(4, 30*time.Millisecond, time.Minute)
	fs := newFakeStreamSession()
	s, err := m.open(context.Background(), "a", &fakeStreamDebugger{sess: fs}, debug.Request{})
	if err != nil {
		t.Fatal(err)
	}
	eventually(t, time.Second, func() bool {
		_, ok := m.get(s.id)
		return !ok
	})
	select {
	case <-fs.closed:
	default:
		t.Fatal("adapter session not closed on idle timeout")
	}
}

func TestSessionManagerTTL(t *testing.T) {
	m := newSessionManager(4, time.Minute, 30*time.Millisecond)
	fs := newFakeStreamSession()
	s, err := m.open(context.Background(), "a", &fakeStreamDebugger{sess: fs}, debug.Request{})
	if err != nil {
		t.Fatal(err)
	}
	eventually(t, time.Second, func() bool {
		_, ok := m.get(s.id)
		return !ok
	})
	select {
	case <-fs.closed:
	default:
		t.Fatal("adapter session not closed on TTL")
	}
}

func TestSessionManagerQueueAndOpenError(t *testing.T) {
	m := newSessionManager(4, time.Minute, time.Minute)
	fs := newFakeStreamSession()
	d := &fakeStreamDebugger{sess: fs}
	s, err := m.open(context.Background(), "a", d, debug.Request{})
	if err != nil {
		t.Fatal(err)
	}
	if cap(s.events) != maxSessionQueue {
		t.Fatalf("queue cap = %d, want %d", cap(s.events), maxSessionQueue)
	}
	m.remove(s.id)

	boom := &fakeStreamDebugger{openErr: fmt.Errorf("dial failed")}
	if _, err := m.open(context.Background(), "a", boom, debug.Request{}); err == nil {
		t.Fatal("open error swallowed")
	}
	if len(m.sessions) != 0 {
		t.Fatalf("failed open left sessions: %d", len(m.sessions))
	}
}

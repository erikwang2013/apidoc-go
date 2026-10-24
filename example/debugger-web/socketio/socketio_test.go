package socketio

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	ws "github.com/gorilla/websocket"

	"github.com/erikwang2013/apidoc-go"
)

type fakeServer struct {
	*httptest.Server
	connects chan string
	pongs    chan struct{}
}

func newFakeServer(t *testing.T, namespace string) *fakeServer {
	t.Helper()
	up := ws.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	fs := &fakeServer{connects: make(chan string, 1), pongs: make(chan struct{}, 1)}
	prefix := ""
	if namespace != "" && namespace != "/" {
		prefix = namespace + ","
	}
	fs.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/socket.io/" {
			t.Errorf("engine path = %q", r.URL.Path)
		}
		if q := r.URL.Query(); q.Get("EIO") != "4" || q.Get("transport") != "websocket" {
			t.Errorf("handshake query = %v", r.URL.Query())
		}
		c, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer c.Close()
		_ = c.WriteMessage(ws.TextMessage, []byte(`0{"sid":"eio","pingInterval":25000,"pingTimeout":20000,"maxPayload":1000000}`))

		_, data, err := c.ReadMessage()
		if err != nil {
			return
		}
		fs.connects <- string(data)
		_ = c.WriteMessage(ws.TextMessage, []byte("40"+prefix+`{"sid":"sio"}`))

		var once sync.Once
		for {
			_, data, err := c.ReadMessage()
			if err != nil {
				return
			}
			pkt := string(data)
			switch {
			case pkt == "2":
				_ = c.WriteMessage(ws.TextMessage, []byte("3"))
			case pkt == "3":
				fs.pongs <- struct{}{}
			case strings.HasPrefix(pkt, "42"+prefix):
				_ = c.WriteMessage(ws.TextMessage, []byte(pkt))
				once.Do(func() { _ = c.WriteMessage(ws.TextMessage, []byte("2")) })
			case strings.HasPrefix(pkt, "41"+prefix):
				return
			}
		}
	}))
	t.Cleanup(fs.Close)
	return fs
}

func hostOf(s *httptest.Server) string {
	return strings.TrimPrefix(s.URL, "http://")
}

func nextEvent(t *testing.T, s apidoc.Session) apidoc.DebugEvent {
	t.Helper()
	select {
	case ev, ok := <-s.Events():
		if !ok {
			t.Fatal("events channel closed")
		}
		return ev
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for event")
		return apidoc.DebugEvent{}
	}
}

func TestSessionDefaultNamespace(t *testing.T) {
	srv := newFakeServer(t, "/")
	s, err := New().Open(context.Background(), apidoc.DebugRequest{URL: "socketio://" + hostOf(srv.Server)})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	select {
	case got := <-srv.connects:
		if got != "40" {
			t.Fatalf("connect packet = %q", got)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no connect packet")
	}

	ev := nextEvent(t, s)
	if ev.Name != "handshake" || !strings.Contains(ev.Data, `"sid":"eio"`) {
		t.Fatalf("handshake event = %+v", ev)
	}
	ev = nextEvent(t, s)
	if ev.Name != "connect" || !strings.Contains(ev.Data, `"sio"`) {
		t.Fatalf("connect event = %+v", ev)
	}

	if err := s.Send(context.Background(), []byte(`["echo",{"n":1}]`), nil); err != nil {
		t.Fatal(err)
	}
	ev = nextEvent(t, s)
	if ev.Name != "echo" || ev.Data != `["echo",{"n":1}]` || ev.Encoding != "json" {
		t.Fatalf("echo event = %+v", ev)
	}

	// The fake server pings once after the first event; the client must pong.
	select {
	case <-srv.pongs:
	case <-time.After(3 * time.Second):
		t.Fatal("client did not answer engine.io ping")
	}

	if err := s.Send(context.Background(), []byte("hello"), map[string]string{"event": "chat"}); err != nil {
		t.Fatal(err)
	}
	ev = nextEvent(t, s)
	if ev.Name != "chat" || ev.Data != `["chat","hello"]` {
		t.Fatalf("named event = %+v", ev)
	}
}

func TestSessionNamespaceAndAuth(t *testing.T) {
	srv := newFakeServer(t, "/chat")
	s, err := New().Open(context.Background(), apidoc.DebugRequest{
		URL:     "socketio://" + hostOf(srv.Server) + "/chat",
		Options: map[string]string{"auth": `{"token":"t"}`},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	select {
	case got := <-srv.connects:
		if got != `40/chat,{"token":"t"}` {
			t.Fatalf("connect packet = %q", got)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no connect packet")
	}
	if ev := nextEvent(t, s); ev.Name != "handshake" {
		t.Fatalf("handshake event = %+v", ev)
	}
	if ev := nextEvent(t, s); ev.Name != "connect" {
		t.Fatalf("connect event = %+v", ev)
	}

	if err := s.Send(context.Background(), []byte(`["chat","yo"]`), nil); err != nil {
		t.Fatal(err)
	}
	ev := nextEvent(t, s)
	if ev.Name != "chat" || ev.Data != `["chat","yo"]` {
		t.Fatalf("namespaced event = %+v", ev)
	}
}

func TestExecOneShot(t *testing.T) {
	srv := newFakeServer(t, "/")
	resp, err := New().Exec(context.Background(), apidoc.DebugRequest{
		URL:     "socketio://" + hostOf(srv.Server),
		Body:    []byte("hi"),
		Options: map[string]string{"timeout_ms": "2000"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Status != "OK" || string(resp.Body) != `["message","hi"]` || resp.Encoding != "json" {
		t.Fatalf("response = %+v", resp)
	}
}

func TestBadAuthJSON(t *testing.T) {
	srv := newFakeServer(t, "/")
	_, err := New().Open(context.Background(), apidoc.DebugRequest{
		URL:     "socketio://" + hostOf(srv.Server),
		Options: map[string]string{"auth": "not-json"},
	})
	if err == nil || !strings.Contains(err.Error(), "auth") {
		t.Fatalf("Open error = %v", err)
	}
}

func TestBadURL(t *testing.T) {
	if _, err := New().Open(context.Background(), apidoc.DebugRequest{URL: "http://x"}); err == nil {
		t.Fatal("want error for unsupported scheme")
	}
}

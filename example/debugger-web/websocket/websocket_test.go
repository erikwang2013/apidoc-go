package websocket

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	ws "github.com/gorilla/websocket"

	"github.com/erikwang2013/apidoc-go"
)

func echoServer(t *testing.T, subprotocols ...string) *httptest.Server {
	t.Helper()
	up := ws.Upgrader{
		Subprotocols: subprotocols,
		CheckOrigin:  func(*http.Request) bool { return true },
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer c.Close()
		for {
			mt, data, err := c.ReadMessage()
			if err != nil {
				return
			}
			if err := c.WriteMessage(mt, data); err != nil {
				return
			}
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func wsURL(srv *httptest.Server) string {
	return "ws" + strings.TrimPrefix(srv.URL, "http")
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

func TestSessionEcho(t *testing.T) {
	srv := echoServer(t)
	s, err := New().Open(context.Background(), apidoc.DebugRequest{URL: wsURL(srv)})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	if err := s.Send(context.Background(), []byte("hello"), nil); err != nil {
		t.Fatal(err)
	}
	ev := nextEvent(t, s)
	if ev.Direction != "in" || ev.Data != "hello" || ev.Encoding != "text" {
		t.Fatalf("text event = %+v", ev)
	}

	if err := s.Send(context.Background(), []byte(`{"a":1}`), nil); err != nil {
		t.Fatal(err)
	}
	ev = nextEvent(t, s)
	if ev.Data != `{"a":1}` || ev.Encoding != "json" {
		t.Fatalf("json event = %+v", ev)
	}

	if err := s.Send(context.Background(), []byte{0, 1, 2, 255}, map[string]string{"encoding": "base64"}); err != nil {
		t.Fatal(err)
	}
	ev = nextEvent(t, s)
	if ev.Data != "AAEC/w==" || ev.Encoding != "base64" {
		t.Fatalf("binary event = %+v", ev)
	}
}

func TestExecOneShot(t *testing.T) {
	srv := echoServer(t)
	resp, err := New().Exec(context.Background(), apidoc.DebugRequest{
		URL:     wsURL(srv),
		Body:    []byte("ping"),
		Options: map[string]string{"read_timeout_ms": "1000"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Status != "OK" || string(resp.Body) != "ping" || resp.Duration <= 0 {
		t.Fatalf("response = %+v", resp)
	}
}

func TestExecSubprotocol(t *testing.T) {
	srv := echoServer(t, "chat")
	resp, err := New().Exec(context.Background(), apidoc.DebugRequest{
		URL:     wsURL(srv),
		Body:    []byte("hi"),
		Options: map[string]string{"subprotocol": "chat", "read_timeout_ms": "1000"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Headers["subprotocol"] != "chat" {
		t.Fatalf("headers = %+v", resp.Headers)
	}
}

func TestExecTimeout(t *testing.T) {
	up := ws.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer c.Close()
		time.Sleep(2 * time.Second)
	}))
	defer srv.Close()

	resp, err := New().Exec(context.Background(), apidoc.DebugRequest{
		URL:     wsURL(srv),
		Options: map[string]string{"read_timeout_ms": "50"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Status != "TIMEOUT" {
		t.Fatalf("status = %q, want TIMEOUT", resp.Status)
	}
}

func TestBadURL(t *testing.T) {
	if _, err := New().Exec(context.Background(), apidoc.DebugRequest{URL: "http://example.com"}); err == nil {
		t.Fatal("want error for non-ws url")
	}
}

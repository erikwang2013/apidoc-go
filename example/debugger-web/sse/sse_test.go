package sse

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/erikwang2013/apidoc-go"
)

func sseServer(t *testing.T, handler func(http.ResponseWriter, *http.Request)) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(handler))
	t.Cleanup(srv.Close)
	return srv
}

func sseURL(srv *httptest.Server) string {
	return "sse" + strings.TrimPrefix(srv.URL, "http")
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

func TestOpenStreamsEvents(t *testing.T) {
	srv := sseServer(t, func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Accept"); got != "text/event-stream" {
			t.Errorf("Accept = %q", got)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		f := w.(http.Flusher)
		fmt.Fprint(w, ": heartbeat comment\n\n")
		fmt.Fprint(w, "event: greeting\nid: 7\ndata: {\"msg\":\"hi\"}\n\n")
		f.Flush()
		fmt.Fprint(w, "data: plain\ndata: second\n\n")
		f.Flush()
	})

	s, err := New().Open(context.Background(), apidoc.DebugRequest{URL: sseURL(srv)})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	ev := nextEvent(t, s)
	if ev.Name != "greeting" || ev.Data != `{"msg":"hi"}` || ev.Encoding != "json" {
		t.Fatalf("first event = %+v", ev)
	}
	ev = nextEvent(t, s)
	if ev.Name != "message" || ev.Data != "plain\nsecond" || ev.Encoding != "text" {
		t.Fatalf("second event = %+v", ev)
	}
	select {
	case _, ok := <-s.Events():
		if ok {
			t.Fatal("expected closed event channel")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("events channel not closed after stream end")
	}
}

// The doc server cancels the Open context as soon as Open returns; the
// stream must keep delivering events after that.
func TestSessionOutlivesOpenContext(t *testing.T) {
	srv := sseServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		f := w.(http.Flusher)
		fmt.Fprint(w, "data: early\n\n")
		f.Flush()
		time.Sleep(50 * time.Millisecond)
		fmt.Fprint(w, "data: late\n\n")
		f.Flush()
	})
	ctx, cancel := context.WithCancel(context.Background())
	s, err := New().Open(ctx, apidoc.DebugRequest{URL: sseURL(srv)})
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	defer s.Close()
	cancel()

	if ev := nextEvent(t, s); ev.Data != "early" {
		t.Fatalf("first event = %+v", ev)
	}
	if ev := nextEvent(t, s); ev.Data != "late" {
		t.Fatalf("second event = %+v, want late", ev)
	}
}

func TestOpenHonorsContext(t *testing.T) {
	block := make(chan struct{})
	srv := sseServer(t, func(w http.ResponseWriter, r *http.Request) {
		<-block
	})
	defer close(block)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err := New().Open(ctx, apidoc.DebugRequest{URL: sseURL(srv)}); err == nil {
		t.Fatal("want error when the open context expires")
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("Open took %s, context was not honored", elapsed)
	}
}

func TestLastEventID(t *testing.T) {
	var got string
	srv := sseServer(t, func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("Last-Event-ID")
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: resumed\n\n")
	})

	if _, err := New().Exec(context.Background(), apidoc.DebugRequest{
		URL:     sseURL(srv),
		Options: map[string]string{"last_event_id": "42"},
	}); err != nil {
		t.Fatal(err)
	}
	if got != "42" {
		t.Fatalf("Last-Event-ID = %q", got)
	}
}

func TestExecOneShot(t *testing.T) {
	srv := sseServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: tick\n\n")
	})

	resp, err := New().Exec(context.Background(), apidoc.DebugRequest{URL: sseURL(srv)})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Status != "OK" || string(resp.Body) != "tick" || len(resp.Events) != 1 {
		t.Fatalf("response = %+v", resp)
	}
}

func TestSendReceiveOnly(t *testing.T) {
	srv := sseServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
	})
	s, err := New().Open(context.Background(), apidoc.DebugRequest{URL: sseURL(srv)})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.Send(context.Background(), []byte("x"), nil); err == nil {
		t.Fatal("want error from Send on an SSE session")
	}
}

func TestOpenHTTPError(t *testing.T) {
	srv := sseServer(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusUnauthorized)
	})
	if _, err := New().Open(context.Background(), apidoc.DebugRequest{URL: sseURL(srv)}); err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("Open error = %v", err)
	}
}

func TestBadURL(t *testing.T) {
	if _, err := New().Open(context.Background(), apidoc.DebugRequest{URL: "ftp://x"}); err == nil {
		t.Fatal("want error for unsupported scheme")
	}
}

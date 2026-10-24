package mqtt

import (
	"context"
	"testing"
	"time"

	mqttserver "github.com/mochi-mqtt/server/v2"
	"github.com/mochi-mqtt/server/v2/hooks/auth"
	"github.com/mochi-mqtt/server/v2/listeners"

	"github.com/erikwang2013/apidoc-go"
)

func newBroker(t *testing.T) string {
	t.Helper()
	server := mqttserver.New(nil)
	if err := server.AddHook(new(auth.AllowHook), nil); err != nil {
		t.Fatal(err)
	}
	tcp := listeners.NewTCP(listeners.Config{ID: "test", Address: "127.0.0.1:0"})
	if err := server.AddListener(tcp); err != nil {
		t.Fatal(err)
	}
	go func() { _ = server.Serve() }()
	t.Cleanup(func() { _ = server.Close() })
	return tcp.Address()
}

func waitIn(t *testing.T, events <-chan apidoc.DebugEvent) apidoc.DebugEvent {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		select {
		case ev, ok := <-events:
			if !ok {
				t.Fatal("events closed before a message arrived")
			}
			if ev.Direction == "in" {
				return ev
			}
		case <-deadline:
			t.Fatal("timed out waiting for event")
			return apidoc.DebugEvent{}
		}
	}
}

func TestSessionPublishSubscribe(t *testing.T) {
	addr := newBroker(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	sess, err := New().Open(ctx, apidoc.DebugRequest{
		Method: "SUBSCRIBE", URL: "mqtt://" + addr + "/sensors/temp",
		Options: map[string]string{"qos": "1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()

	if err := sess.Send(ctx, []byte("32312e35"), map[string]string{"encoding": "hex"}); err != nil {
		t.Fatal(err)
	}
	ev := waitIn(t, sess.Events())
	if ev.Direction != "in" || ev.Data != "21.5" || ev.Name != "sensors/temp" || ev.Encoding != "text" {
		t.Fatalf("event = %+v", ev)
	}
}

func TestExecPublish(t *testing.T) {
	addr := newBroker(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	sess, err := New().Open(ctx, apidoc.DebugRequest{
		Method: "SUBSCRIBE", URL: "mqtt://" + addr + "/exec/topic",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()

	resp, err := New().Exec(ctx, apidoc.DebugRequest{
		Method: "PUBLISH", URL: "mqtt://" + addr + "/exec/topic",
		Body: []byte("hello"), Encoding: "text",
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Status != "PUBLISHED" || resp.Headers["topic"] != "exec/topic" || resp.Headers["qos"] != "0" {
		t.Fatalf("response = %+v", resp)
	}
	ev := waitIn(t, sess.Events())
	if ev.Data != "hello" {
		t.Fatalf("event = %+v", ev)
	}
}

func TestExecRejectsSubscribeMethod(t *testing.T) {
	if _, err := New().Exec(context.Background(), apidoc.DebugRequest{
		Method: "SUBSCRIBE", URL: "mqtt://127.0.0.1:1/topic",
	}); err == nil {
		t.Fatal("want error for SUBSCRIBE via Exec")
	}
}

func TestBadURL(t *testing.T) {
	if _, err := New().Exec(context.Background(), apidoc.DebugRequest{
		Method: "PUBLISH", URL: "http://127.0.0.1:1/topic",
	}); err == nil {
		t.Fatal("want error for non-mqtt url")
	}
	if _, err := New().Open(context.Background(), apidoc.DebugRequest{
		URL: "mqtt://127.0.0.1:1",
	}); err == nil {
		t.Fatal("want error for mqtt url without topic")
	}
}

func TestSessionCloseClosesEvents(t *testing.T) {
	addr := newBroker(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	sess, err := New().Open(ctx, apidoc.DebugRequest{URL: "mqtt://" + addr + "/close/topic"})
	if err != nil {
		t.Fatal(err)
	}
	if err := sess.Close(); err != nil {
		t.Fatal(err)
	}
	deadline := time.After(2 * time.Second)
	for {
		select {
		case _, ok := <-sess.Events():
			if !ok {
				return
			}
		case <-deadline:
			t.Fatal("events channel not closed after Close")
		}
	}
}

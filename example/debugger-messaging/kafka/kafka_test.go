package kafka

import (
	"context"
	"testing"
	"time"

	"github.com/twmb/franz-go/pkg/kfake"
	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/erikwang2013/apidoc-go"
)

func newCluster(t *testing.T, topics ...string) string {
	t.Helper()
	cluster, err := kfake.NewCluster(kfake.NumBrokers(1), kfake.SeedTopics(1, topics...))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cluster.Close)
	return cluster.ListenAddrs()[0]
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

func TestExecProduce(t *testing.T) {
	addr := newCluster(t, "exec-topic")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resp, err := New().Exec(ctx, apidoc.DebugRequest{
		Method: "PRODUCE", URL: "kafka://" + addr + "/exec-topic",
		Body: []byte("one"), Encoding: "text",
		Headers: map[string]string{"trace": "abc"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Status != "PRODUCED" || resp.Headers["topic"] != "exec-topic" ||
		resp.Headers["partition"] != "0" || resp.Headers["offset"] == "" {
		t.Fatalf("response = %+v", resp)
	}

	consumer, err := kgo.NewClient(kgo.SeedBrokers(addr), kgo.ConsumeTopics("exec-topic"))
	if err != nil {
		t.Fatal(err)
	}
	defer consumer.Close()
	fctx, fcancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer fcancel()
	var records []*kgo.Record
	consumer.PollFetches(fctx).EachRecord(func(r *kgo.Record) { records = append(records, r) })
	if len(records) != 1 || string(records[0].Value) != "one" {
		t.Fatalf("records = %+v", records)
	}
	if len(records[0].Headers) != 1 || records[0].Headers[0].Key != "trace" ||
		string(records[0].Headers[0].Value) != "abc" {
		t.Fatalf("headers = %+v", records[0].Headers)
	}
}

func TestSessionConsumeAndProduce(t *testing.T) {
	addr := newCluster(t, "session-topic")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	sess, err := New().Open(ctx, apidoc.DebugRequest{
		Method: "CONSUME", URL: "kafka://" + addr + "/session-topic",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()

	if err := sess.Send(ctx, []byte("74776f"), map[string]string{"encoding": "hex"}); err != nil {
		t.Fatal(err)
	}
	ev := waitIn(t, sess.Events())
	if ev.Data != "two" || ev.Name != "session-topic" || ev.Encoding != "text" {
		t.Fatalf("event = %+v", ev)
	}
}

func TestSessionCloseClosesEvents(t *testing.T) {
	addr := newCluster(t, "close-topic")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	sess, err := New().Open(ctx, apidoc.DebugRequest{URL: "kafka://" + addr + "/close-topic"})
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

func TestExecRejectsConsumeMethod(t *testing.T) {
	if _, err := New().Exec(context.Background(), apidoc.DebugRequest{
		Method: "CONSUME", URL: "kafka://127.0.0.1:1/topic",
	}); err == nil {
		t.Fatal("want error for CONSUME via Exec")
	}
}

func TestBadURL(t *testing.T) {
	if _, err := New().Exec(context.Background(), apidoc.DebugRequest{
		Method: "PRODUCE", URL: "http://127.0.0.1:1/topic",
	}); err == nil {
		t.Fatal("want error for non-kafka url")
	}
}

func TestHeadersSkipEncoding(t *testing.T) {
	hs := headers(map[string]string{"encoding": "hex", "trace": "abc"})
	if len(hs) != 1 || hs[0].Key != "trace" {
		t.Fatalf("headers = %+v", hs)
	}
}

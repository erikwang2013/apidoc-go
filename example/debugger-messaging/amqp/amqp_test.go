package amqp

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	amqp091 "github.com/rabbitmq/amqp091-go"

	"github.com/erikwang2013/apidoc-go"
)

// fakeChannel is an in-memory AMQP channel: it records declares/publishes
// and hands test deliveries to the consumer. No broker process is needed.
type fakeChannel struct {
	mu            sync.Mutex
	existing      map[string]bool
	declared      []string
	published     []published
	deliveries    chan amqp091.Delivery
	consumedQueue string
	autoAck       bool
	closed        bool
}

type published struct {
	exchange   string
	routingKey string
	msg        amqp091.Publishing
}

func (f *fakeChannel) QueueDeclarePassive(name string, durable, autoDelete, exclusive, noWait bool, args amqp091.Table) (amqp091.Queue, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.existing[name] {
		return amqp091.Queue{Name: name}, nil
	}
	return amqp091.Queue{}, errors.New("404 NOT_FOUND")
}

func (f *fakeChannel) QueueDeclare(name string, durable, autoDelete, exclusive, noWait bool, args amqp091.Table) (amqp091.Queue, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.declared = append(f.declared, name)
	return amqp091.Queue{Name: name}, nil
}

func (f *fakeChannel) PublishWithContext(ctx context.Context, exchange, key string, mandatory, immediate bool, msg amqp091.Publishing) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.published = append(f.published, published{exchange: exchange, routingKey: key, msg: msg})
	return nil
}

func (f *fakeChannel) ConsumeWithContext(ctx context.Context, queue, consumer string, autoAck, exclusive, noLocal, noWait bool, args amqp091.Table) (<-chan amqp091.Delivery, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.consumedQueue, f.autoAck = queue, autoAck
	return f.deliveries, nil
}

func (f *fakeChannel) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = true
	return nil
}

func (f *fakeChannel) snapshot() (declared []string, publishes []published, queue string, autoAck, closed bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.declared...), append([]published(nil), f.published...),
		f.consumedQueue, f.autoAck, f.closed
}

type fakeConn struct{ ch *fakeChannel }

func (c *fakeConn) Channel() (amqpChannel, error) { return c.ch, nil }
func (c *fakeConn) Close() error                  { return nil }

type fakeAcker struct {
	mu    sync.Mutex
	acked bool
}

func (a *fakeAcker) Ack(tag uint64, multiple bool) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.acked = true
	return nil
}
func (a *fakeAcker) Nack(tag uint64, multiple, requeue bool) error { return nil }
func (a *fakeAcker) Reject(tag uint64, requeue bool) error         { return nil }

func (a *fakeAcker) isAcked() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.acked
}

func newFakeDebugger(fake *fakeChannel, dialed *string) *debugger {
	return &debugger{dial: func(raw string) (amqpConn, error) {
		if dialed != nil {
			*dialed = raw
		}
		return &fakeConn{ch: fake}, nil
	}}
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

func TestExecPublish(t *testing.T) {
	fake := &fakeChannel{}
	var dialed string
	d := newFakeDebugger(fake, &dialed)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resp, err := d.Exec(ctx, apidoc.DebugRequest{
		Method: "PUBLISH", URL: "amqp://user:pass@broker:5672/jobs",
		Body: []byte("hi"), Encoding: "text",
		Options: map[string]string{"vhost": "vh", "exchange": "ex", "routing_key": "rk"},
		Headers: map[string]string{"content-type": "application/json"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Status != "PUBLISHED" || resp.Headers["exchange"] != "ex" || resp.Headers["routing_key"] != "rk" {
		t.Fatalf("response = %+v", resp)
	}
	if dialed != "amqp://user:pass@broker:5672/vh" {
		t.Fatalf("dialed = %q", dialed)
	}
	_, publishes, _, _, _ := fake.snapshot()
	if len(publishes) != 1 {
		t.Fatalf("publishes = %+v", publishes)
	}
	got := publishes[0]
	if got.exchange != "ex" || got.routingKey != "rk" || string(got.msg.Body) != "hi" ||
		got.msg.ContentType != "application/json" {
		t.Fatalf("publish = %+v", got)
	}
}

func TestSessionConsumeAndSend(t *testing.T) {
	fake := &fakeChannel{
		existing:   map[string]bool{"jobs": true},
		deliveries: make(chan amqp091.Delivery, 1),
	}
	d := newFakeDebugger(fake, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	sess, err := d.Open(ctx, apidoc.DebugRequest{
		URL: "amqp://broker:5672/jobs", Options: map[string]string{"auto_ack": "false"},
	})
	if err != nil {
		t.Fatal(err)
	}
	declared, _, queue, autoAck, _ := fake.snapshot()
	if queue != "jobs" || autoAck {
		t.Fatalf("consume queue=%q autoAck=%v", queue, autoAck)
	}
	if len(declared) != 0 {
		t.Fatalf("existing queue must not be declared again: %v", declared)
	}

	acker := &fakeAcker{}
	fake.deliveries <- amqp091.Delivery{
		Acknowledger: acker, Body: []byte("in"), RoutingKey: "jobs",
	}
	ev := waitIn(t, sess.Events())
	if ev.Data != "in" || ev.Name != "jobs" {
		t.Fatalf("event = %+v", ev)
	}
	deadline := time.Now().Add(time.Second)
	for !acker.isAcked() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if !acker.isAcked() {
		t.Fatal("delivery was not acked")
	}

	if err := sess.Send(ctx, []byte("6f7574"), map[string]string{"encoding": "hex"}); err != nil {
		t.Fatal(err)
	}
	_, publishes, _, _, _ := fake.snapshot()
	if len(publishes) != 1 || publishes[0].exchange != "" ||
		publishes[0].routingKey != "jobs" || string(publishes[0].msg.Body) != "out" {
		t.Fatalf("publish = %+v", publishes)
	}
	if publishes[0].msg.Headers["encoding"] != nil {
		t.Fatalf("encoding leaked into headers: %v", publishes[0].msg.Headers)
	}

	if err := sess.Close(); err != nil {
		t.Fatal(err)
	}
	closeDeadline := time.After(2 * time.Second)
	for {
		select {
		case _, ok := <-sess.Events():
			if !ok {
				return
			}
		case <-closeDeadline:
			t.Fatal("events channel not closed after Close")
		}
	}
}

func TestOpenDeclaresMissingQueue(t *testing.T) {
	fake := &fakeChannel{deliveries: make(chan amqp091.Delivery)}
	d := newFakeDebugger(fake, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	sess, err := d.Open(ctx, apidoc.DebugRequest{URL: "amqp://broker:5672/newq"})
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()
	declared, _, _, _, _ := fake.snapshot()
	if len(declared) != 1 || declared[0] != "newq" {
		t.Fatalf("declared = %v", declared)
	}
}

func TestExecRejectsConsumeMethod(t *testing.T) {
	if _, err := newFakeDebugger(&fakeChannel{}, nil).Exec(context.Background(), apidoc.DebugRequest{
		Method: "CONSUME", URL: "amqp://127.0.0.1:1/jobs",
	}); err == nil {
		t.Fatal("want error for CONSUME via Exec")
	}
}

func TestBadURL(t *testing.T) {
	if _, err := newFakeDebugger(&fakeChannel{}, nil).Exec(context.Background(), apidoc.DebugRequest{
		Method: "PUBLISH", URL: "http://127.0.0.1:1/jobs",
	}); err == nil {
		t.Fatal("want error for non-amqp url")
	}
	if _, err := newFakeDebugger(&fakeChannel{}, nil).Open(context.Background(), apidoc.DebugRequest{
		URL: "amqp://127.0.0.1:1",
	}); err == nil {
		t.Fatal("want error for amqp url without queue")
	}
}

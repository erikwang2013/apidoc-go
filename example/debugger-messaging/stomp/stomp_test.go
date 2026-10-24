package stomp

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/erikwang2013/apidoc-go"
)

// testBroker is a minimal STOMP 1.2 server: CONNECT, SUBSCRIBE, SEND,
// UNSUBSCRIBE and DISCONNECT with RECEIPT support. SEND is broadcast to
// every subscription on the same destination.
type testBroker struct {
	ln    net.Listener
	mu    sync.Mutex
	subs  map[string]map[*testConn]string
	sent  chan sentMessage
	msgID atomic.Int64
}

type sentMessage struct {
	destination string
	body        []byte
}

type testConn struct {
	conn   net.Conn
	w      *bufio.Writer
	writeM sync.Mutex
}

func newTestBroker(t *testing.T) *testBroker {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	b := &testBroker{
		ln:   ln,
		subs: map[string]map[*testConn]string{},
		sent: make(chan sentMessage, 16),
	}
	t.Cleanup(func() { ln.Close() })
	go b.accept()
	return b
}

func (b *testBroker) addr() string { return b.ln.Addr().String() }

func (b *testBroker) accept() {
	for {
		conn, err := b.ln.Accept()
		if err != nil {
			return
		}
		go b.serve(conn)
	}
}

func (b *testBroker) addSub(destination string, c *testConn, id string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.subs[destination] == nil {
		b.subs[destination] = map[*testConn]string{}
	}
	b.subs[destination][c] = id
}

func (b *testBroker) removeSub(c *testConn, id string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for destination, conns := range b.subs {
		if subID, ok := conns[c]; ok && subID == id {
			delete(conns, c)
			if len(conns) == 0 {
				delete(b.subs, destination)
			}
		}
	}
}

func (b *testBroker) removeConn(c *testConn) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for destination, conns := range b.subs {
		delete(conns, c)
		if len(conns) == 0 {
			delete(b.subs, destination)
		}
	}
}

type recipient struct {
	conn *testConn
	id   string
}

func (b *testBroker) recipients(destination string) []recipient {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]recipient, 0, len(b.subs[destination]))
	for conn, id := range b.subs[destination] {
		out = append(out, recipient{conn: conn, id: id})
	}
	return out
}

func (c *testConn) writeFrame(command string, headers map[string]string, body []byte) error {
	c.writeM.Lock()
	defer c.writeM.Unlock()
	var sb strings.Builder
	sb.WriteString(command)
	sb.WriteByte('\n')
	for k, v := range headers {
		sb.WriteString(k)
		sb.WriteByte(':')
		sb.WriteString(v)
		sb.WriteByte('\n')
	}
	fmt.Fprintf(&sb, "content-length:%d\n\n", len(body))
	if _, err := c.w.WriteString(sb.String()); err != nil {
		return err
	}
	if _, err := c.w.Write(body); err != nil {
		return err
	}
	if err := c.w.WriteByte(0); err != nil {
		return err
	}
	return c.w.Flush()
}

func readFrame(r *bufio.Reader) (command string, headers map[string]string, body []byte, err error) {
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return "", nil, nil, err
		}
		command = strings.TrimRight(line, "\r\n")
		if command != "" {
			break
		}
	}
	headers = map[string]string{}
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return "", nil, nil, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		key, value, ok := strings.Cut(line, ":")
		if ok {
			headers[strings.TrimSpace(key)] = strings.TrimSpace(value)
		}
	}
	if size, err := strconv.Atoi(headers["content-length"]); err == nil {
		body = make([]byte, size)
		if _, err := io.ReadFull(r, body); err != nil {
			return "", nil, nil, err
		}
		nul, err := r.ReadByte()
		if err != nil {
			return "", nil, nil, err
		}
		if nul != 0 {
			return "", nil, nil, fmt.Errorf("missing frame terminator")
		}
		return command, headers, body, nil
	}
	raw, err := r.ReadBytes(0)
	if err != nil {
		return "", nil, nil, err
	}
	return command, headers, raw[:len(raw)-1], nil
}

func (b *testBroker) serve(nc net.Conn) {
	c := &testConn{conn: nc, w: bufio.NewWriter(nc)}
	defer func() {
		nc.Close()
		b.removeConn(c)
	}()
	reader := bufio.NewReader(nc)
	for {
		command, headers, body, err := readFrame(reader)
		if err != nil {
			return
		}
		receipt := headers["receipt"]
		switch command {
		case "CONNECT", "STOMP":
			if err := c.writeFrame("CONNECTED", map[string]string{
				"version": "1.2", "heart-beat": "0,0",
			}, nil); err != nil {
				return
			}
		case "SUBSCRIBE":
			b.addSub(headers["destination"], c, headers["id"])
		case "UNSUBSCRIBE":
			b.removeSub(c, headers["id"])
		case "SEND":
			destination := headers["destination"]
			for _, r := range b.recipients(destination) {
				messageHeaders := map[string]string{
					"subscription": r.id,
					"destination":  destination,
					"message-id":   fmt.Sprintf("m%d", b.msgID.Add(1)),
				}
				if ct := headers["content-type"]; ct != "" {
					messageHeaders["content-type"] = ct
				}
				if err := r.conn.writeFrame("MESSAGE", messageHeaders, body); err != nil {
					return
				}
			}
			select {
			case b.sent <- sentMessage{destination: destination, body: body}:
			default:
			}
		case "DISCONNECT":
			if receipt != "" {
				_ = c.writeFrame("RECEIPT", map[string]string{"receipt-id": receipt}, nil)
			}
			return
		}
		if receipt != "" && command != "DISCONNECT" {
			if err := c.writeFrame("RECEIPT", map[string]string{"receipt-id": receipt}, nil); err != nil {
				return
			}
		}
	}
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

func TestSessionSendReceive(t *testing.T) {
	broker := newTestBroker(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	sess, err := New().Open(ctx, apidoc.DebugRequest{
		Method: "SUBSCRIBE", URL: "stomp://" + broker.addr() + "/queue/demo",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()

	if err := sess.Send(ctx, []byte("68656c6c6f"), map[string]string{"encoding": "hex"}); err != nil {
		t.Fatal(err)
	}
	ev := waitIn(t, sess.Events())
	if ev.Data != "hello" || ev.Name != "/queue/demo" || ev.Encoding != "text" {
		t.Fatalf("event = %+v", ev)
	}
}

func TestExecSend(t *testing.T) {
	broker := newTestBroker(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resp, err := New().Exec(ctx, apidoc.DebugRequest{
		Method: "SEND", URL: "stomp://" + broker.addr() + "/queue/out",
		Body: []byte("bye"), Encoding: "text",
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Status != "SENT" || resp.Headers["destination"] != "/queue/out" {
		t.Fatalf("response = %+v", resp)
	}
	select {
	case sent := <-broker.sent:
		if sent.destination != "/queue/out" || string(sent.body) != "bye" {
			t.Fatalf("sent = %+v", sent)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("broker did not receive SEND")
	}
}

func TestSessionCloseClosesEvents(t *testing.T) {
	broker := newTestBroker(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	sess, err := New().Open(ctx, apidoc.DebugRequest{
		URL: "stomp://" + broker.addr() + "/queue/close",
	})
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

func TestExecRejectsSubscribeMethod(t *testing.T) {
	if _, err := New().Exec(context.Background(), apidoc.DebugRequest{
		Method: "SUBSCRIBE", URL: "stomp://127.0.0.1:1/queue/x",
	}); err == nil {
		t.Fatal("want error for SUBSCRIBE via Exec")
	}
}

func TestBadURL(t *testing.T) {
	if _, err := New().Exec(context.Background(), apidoc.DebugRequest{
		Method: "SEND", URL: "http://127.0.0.1:1/queue/x",
	}); err == nil {
		t.Fatal("want error for non-stomp url")
	}
	if _, err := New().Open(context.Background(), apidoc.DebugRequest{
		URL: "stomp://127.0.0.1:1",
	}); err == nil {
		t.Fatal("want error for stomp url without destination")
	}
}

//go:build apidoc_debug_amqp || apidoc_debug_messaging || apidoc_debug_all

package debug

import (
	"encoding/binary"
	"io"
	"net"
	"strings"
	"sync"
	"testing"
	"time"
)

type amqpTestServer struct {
	addr    string
	deliver []string
	// stall blocks the named setup exchange ("confirm", "declare", "consume").
	stall     string
	closeText string

	mu        sync.Mutex
	sasl      string
	published []byte
	routing   string
	exchange  string
}

func newAMQPTestServer(t *testing.T, deliver []string) *amqpTestServer {
	t.Helper()
	ln := listenLocal(t)
	srv := &amqpTestServer{addr: ln.Addr().String(), deliver: deliver}
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go srv.handle(conn)
		}
	}()
	return srv
}

func amqpTestWriteFrame(conn net.Conn, frameType byte, channel uint16, payload []byte) error {
	frame := make([]byte, 8+len(payload))
	frame[0] = frameType
	binary.BigEndian.PutUint16(frame[1:], channel)
	binary.BigEndian.PutUint32(frame[3:], uint32(len(payload)))
	copy(frame[7:], payload)
	frame[len(frame)-1] = amqpFrameEnd
	_, err := conn.Write(frame)
	return err
}

func amqpTestWriteMethod(conn net.Conn, channel, class, method uint16, args ...[]byte) error {
	return amqpTestWriteFrame(conn, amqpFrameMethod, channel, amqpMethod(class, method, args...))
}

func amqpTestReadFrame(conn net.Conn) (byte, uint16, []byte, error) {
	head := make([]byte, 7)
	if _, err := io.ReadFull(conn, head); err != nil {
		return 0, 0, nil, err
	}
	size := int(binary.BigEndian.Uint32(head[3:]))
	payload := make([]byte, size)
	if _, err := io.ReadFull(conn, payload); err != nil {
		return 0, 0, nil, err
	}
	end := make([]byte, 1)
	if _, err := io.ReadFull(conn, end); err != nil || end[0] != amqpFrameEnd {
		return 0, 0, nil, io.ErrUnexpectedEOF
	}
	return head[0], binary.BigEndian.Uint16(head[1:]), payload, nil
}

func amqpTestReadMethod(conn net.Conn) (uint16, uint16, uint16, []byte, error) {
	for {
		frameType, channel, payload, err := amqpTestReadFrame(conn)
		if err != nil {
			return 0, 0, 0, nil, err
		}
		if frameType != amqpFrameMethod {
			continue
		}
		class, method, args, err := amqpMethodOf(payload)
		return class, method, channel, args, err
	}
}

func (s *amqpTestServer) handle(conn net.Conn) {
	defer conn.Close()
	header := make([]byte, 8)
	if _, err := io.ReadFull(conn, header); err != nil || string(header) != "AMQP\x00\x00\x09\x01" {
		return
	}
	_ = amqpTestWriteMethod(conn, 0, 10, 10,
		[]byte{0, 9}, amqpEmptyTable(), amqpLongString("PLAIN"), amqpLongString("en_US"))
	_, method, _, args, err := amqpTestReadMethod(conn) // Start-Ok
	if err != nil || method != 11 {
		return
	}
	// args: client-properties table, mechanism shortstr, response longstr...
	pos := amqpSkipTable(args, 0)
	_, pos = amqpShortStringAt(args, pos)
	response, _ := amqpLongStringAt(args, pos)
	s.mu.Lock()
	s.sasl = response
	s.mu.Unlock()

	if s.closeText != "" {
		_ = amqpTestWriteMethod(conn, 0, 10, 50,
			[]byte{0, 200}, amqpShortString(s.closeText), []byte{0, 0}, []byte{0, 0})
		return
	}
	_ = amqpTestWriteMethod(conn, 0, 10, 30, []byte{0, 0}, uint32Bytes(131072), []byte{0, 0})
	if _, method, _, _, err = amqpTestReadMethod(conn); err != nil || method != 31 { // Tune-Ok
		return
	}
	if _, method, _, _, err = amqpTestReadMethod(conn); err != nil || method != 40 { // Open
		return
	}
	_ = amqpTestWriteMethod(conn, 0, 10, 41, amqpShortString(""))
	if _, method, _, _, err = amqpTestReadMethod(conn); err != nil || method != 10 { // Channel.Open
		return
	}
	_ = amqpTestWriteMethod(conn, 1, 20, 11, amqpLongString(""))

	for {
		class, method, channel, args, err := amqpTestReadMethod(conn)
		if err != nil {
			return
		}
		if s.stall == "confirm" && class == 85 && method == 10 ||
			s.stall == "declare" && class == 50 && method == 10 ||
			s.stall == "consume" && class == 60 && method == 20 {
			_, _ = io.Copy(io.Discard, conn) // accept the request, never reply
			return
		}
		switch {
		case class == 10 && method == 50, class == 20 && method == 40:
			return // client closing
		case class == 85 && method == 10:
			_ = amqpTestWriteMethod(conn, channel, 85, 11)
		case class == 50 && method == 10:
			queue, _ := amqpShortStringAt(args, 2)
			_ = amqpTestWriteMethod(conn, channel, 50, 11,
				amqpShortString(queue), []byte{0, 0, 0, 0}, []byte{0, 0, 0, 0})
		case class == 60 && method == 20:
			_, pos := amqpShortStringAt(args, 2)
			tag, _ := amqpShortStringAt(args, pos)
			_ = amqpTestWriteMethod(conn, channel, 60, 21, amqpShortString(tag))
			for i, body := range s.deliver {
				_ = amqpTestWriteMethod(conn, channel, 60, 60,
					amqpShortString(tag),
					uint64Bytes(uint64(i+1)),
					[]byte{0},
					amqpShortString(""),
					amqpShortString("rk"),
				)
				content := make([]byte, 14)
				binary.BigEndian.PutUint16(content, 60)
				binary.BigEndian.PutUint64(content[4:], uint64(len(body)))
				_ = amqpTestWriteFrame(conn, amqpFrameHeader, channel, content)
				_ = amqpTestWriteFrame(conn, amqpFrameBody, channel, []byte(body))
			}
		case class == 60 && method == 40:
			exchange, pos := amqpShortStringAt(args, 2)
			routing, _ := amqpShortStringAt(args, pos)
			frameType, _, payload, err := amqpTestReadFrame(conn)
			if err != nil || frameType != amqpFrameHeader || len(payload) < 14 {
				return
			}
			bodySize := int(binary.BigEndian.Uint64(payload[4:]))
			body := make([]byte, 0, bodySize)
			for len(body) < bodySize {
				frameType, _, chunk, err := amqpTestReadFrame(conn)
				if err != nil || frameType != amqpFrameBody {
					return
				}
				body = append(body, chunk...)
			}
			s.mu.Lock()
			s.exchange, s.routing, s.published = exchange, routing, body
			s.mu.Unlock()
			_ = amqpTestWriteMethod(conn, channel, 60, 80, uint64Bytes(1))
		}
	}
}

func uint64Bytes(value uint64) []byte {
	out := make([]byte, 8)
	binary.BigEndian.PutUint64(out, value)
	return out
}

func amqpTestRequest(addr string, opts map[string]string, body string) *Request {
	all := map[string]string{}
	for key, value := range opts {
		all[key] = value
	}
	req := &Request{Kind: "amqp", Target: "amqp://" + addr, Opts: all}
	if body != "" {
		req.Body = &Payload{Encoding: "utf8", Data: body}
	}
	return req
}

func TestAMQPPublishConfirmed(t *testing.T) {
	srv := newAMQPTestServer(t, nil)
	resp, err := Execute(messagingConfig(), amqpTestRequest(srv.addr, map[string]string{
		"exchange": "events", "routing_key": "orders.created", "username": "u", "password": "p",
	}, "hello"))
	if err != nil {
		t.Fatal(err)
	}
	if body := decodeMessagingJSON(t, resp); body["published"] != true {
		t.Fatalf("body = %#v", body)
	}
	srv.mu.Lock()
	published, routing, exchange, sasl := append([]byte{}, srv.published...), srv.routing, srv.exchange, srv.sasl
	srv.mu.Unlock()
	if string(published) != "hello" || routing != "orders.created" || exchange != "events" {
		t.Fatalf("published = %q %q %q", published, routing, exchange)
	}
	if sasl != "\x00u\x00p" {
		t.Fatalf("sasl = %q", sasl)
	}
}

func TestAMQPConsume(t *testing.T) {
	srv := newAMQPTestServer(t, []string{"first", "second"})
	resp, err := Execute(messagingConfig(), amqpTestRequest(srv.addr, map[string]string{
		"consume": "true", "queue": "jobs", "declare": "true", "messages": "2", "duration_ms": "500",
	}, ""))
	if err != nil {
		t.Fatal(err)
	}
	body := decodeMessagingJSON(t, resp)
	deliveries := body["deliveries"].([]any)
	if len(deliveries) != 2 {
		t.Fatalf("deliveries = %#v", deliveries)
	}
	first := deliveries[0].(map[string]any)
	if first["routing_key"] != "rk" || messagingPayload(first)["data"] != "first" {
		t.Fatalf("delivery = %#v", first)
	}
	if resp.Headers["x-messages"] != "2" {
		t.Fatalf("headers = %#v", resp.Headers)
	}

	single := newAMQPTestServer(t, []string{"only"})
	resp, err = Execute(messagingConfig(), amqpTestRequest(single.addr, map[string]string{
		"consume": "true", "queue": "jobs", "duration_ms": "200",
	}, ""))
	if err != nil {
		t.Fatal(err)
	}
	if delivery, ok := decodeMessagingJSON(t, resp)["delivery"].(map[string]any); !ok || messagingPayload(delivery)["data"] != "only" {
		t.Fatalf("single delivery = %#v", delivery)
	}
}

func TestAMQPWindowCapsSSRFAndValidation(t *testing.T) {
	empty := newAMQPTestServer(t, nil)
	started := time.Now()
	resp, err := Execute(messagingConfig(), amqpTestRequest(empty.addr, map[string]string{
		"consume": "true", "queue": "q", "duration_ms": "40",
	}, ""))
	if err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("empty window took %v", elapsed)
	}
	if deliveries := decodeMessagingJSON(t, resp)["deliveries"].([]any); len(deliveries) != 0 {
		t.Fatalf("deliveries = %#v", deliveries)
	}

	big := newAMQPTestServer(t, []string{strings.Repeat("x", 100)})
	cfg := messagingConfig()
	cfg.MaxBytes = 64
	_, err = Execute(cfg, amqpTestRequest(big.addr, map[string]string{"consume": "true", "queue": "q", "messages": "1"}, ""))
	if err == nil || !strings.Contains(err.Error(), "max_bytes") {
		t.Fatalf("cap err = %v", err)
	}

	private := Config{EnableServerSide: true, TimeoutMS: 500}
	_, err = Execute(private, amqpTestRequest(empty.addr, map[string]string{"consume": "true", "queue": "q"}, ""))
	if err == nil || !strings.Contains(err.Error(), "private") {
		t.Fatalf("ssrf err = %v", err)
	}

	for name, req := range map[string]*Request{
		"publish without routing key": {Kind: "amqp", Target: "amqp://127.0.0.1:1", Opts: map[string]string{"exchange": ""}},
		"consume without queue":       {Kind: "amqp", Target: "amqp://127.0.0.1:1", Opts: map[string]string{"consume": "true"}},
		"invalid declare":             {Kind: "amqp", Target: "amqp://127.0.0.1:1", Opts: map[string]string{"consume": "true", "queue": "q", "declare": "yes"}},
		"tls":                         {Kind: "amqp", Target: "amqps://127.0.0.1:1", Opts: map[string]string{}},
	} {
		if _, err := Execute(messagingConfig(), req); err == nil {
			t.Fatalf("%s was accepted", name)
		}
	}

	long := strings.Repeat("x", 256)
	for name, req := range map[string]*Request{
		"long routing key": {Kind: "amqp", Target: "amqp://127.0.0.1:1", Opts: map[string]string{"routing_key": long}},
		"long exchange":    {Kind: "amqp", Target: "amqp://127.0.0.1:1", Opts: map[string]string{"routing_key": "rk", "exchange": long}},
		"long queue":       {Kind: "amqp", Target: "amqp://127.0.0.1:1", Opts: map[string]string{"consume": "true", "queue": long}},
		"long vhost":       {Kind: "amqp", Target: "amqp://127.0.0.1:1", Opts: map[string]string{"routing_key": "rk", "vhost": long}},
	} {
		if _, err := Execute(messagingConfig(), req); err == nil || !strings.Contains(err.Error(), "255") {
			t.Fatalf("%s err = %v", name, err)
		}
	}
}

func TestAMQPSetupTimeout(t *testing.T) {
	cfg := messagingConfig()
	cfg.TimeoutMS = 200
	started := time.Now()
	publish := newAMQPTestServer(t, nil)
	publish.stall = "confirm"
	if _, err := Execute(cfg, amqpTestRequest(publish.addr, map[string]string{"routing_key": "rk"}, "x")); err == nil {
		t.Fatal("stalled publish confirmation was accepted")
	}
	if elapsed := time.Since(started); elapsed > 2*time.Second {
		t.Fatalf("stalled publish took %v", elapsed)
	}

	consume := newAMQPTestServer(t, nil)
	consume.stall = "consume"
	started = time.Now()
	if _, err := Execute(cfg, amqpTestRequest(consume.addr, map[string]string{"consume": "true", "queue": "q"}, "")); err == nil {
		t.Fatal("stalled consume setup was accepted")
	}
	if elapsed := time.Since(started); elapsed > 2*time.Second {
		t.Fatalf("stalled consume took %v", elapsed)
	}

	stream := newAMQPTestServer(t, nil)
	stream.stall = "declare"
	started = time.Now()
	if _, err := OpenStream(cfg, amqpTestRequest(stream.addr, map[string]string{"consume": "true", "queue": "q", "declare": "true"}, "")); err == nil {
		t.Fatal("stalled stream setup was accepted")
	}
	if elapsed := time.Since(started); elapsed > 2*time.Second {
		t.Fatalf("stalled stream setup took %v", elapsed)
	}
}

func TestAMQPErrorRedaction(t *testing.T) {
	srv := newAMQPTestServer(t, nil)
	srv.closeText = "access denied for pw-secret"
	_, err := Execute(messagingConfig(), amqpTestRequest(srv.addr, map[string]string{
		"routing_key": "rk", "password": "pw-secret",
	}, "x"))
	if err == nil || !strings.Contains(err.Error(), "***") {
		t.Fatalf("close err = %v", err)
	}
	if strings.Contains(err.Error(), "pw-secret") {
		t.Fatalf("close error leaked the password: %v", err)
	}
}

func TestAMQPStreamSession(t *testing.T) {
	srv := newAMQPTestServer(t, []string{"one", "two"})
	stream, err := OpenStream(messagingConfig(), amqpTestRequest(srv.addr, map[string]string{"consume": "true", "queue": "q"}, ""))
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	frame, err := stream.ReadFrame()
	if err != nil || string(frame) != "one" {
		t.Fatalf("first = %q err=%v", frame, err)
	}
	frame, err = stream.ReadFrame()
	if err != nil || string(frame) != "two" {
		t.Fatalf("second = %q err=%v", frame, err)
	}
	if err := stream.Close(); err != nil {
		t.Fatal(err)
	}
}

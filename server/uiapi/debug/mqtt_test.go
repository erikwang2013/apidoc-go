//go:build apidoc_debug_mqtt || apidoc_debug_messaging || apidoc_debug_all

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

type mqttTestServer struct {
	ln      net.Listener
	deliver [][]byte
	echo    bool

	mu        sync.Mutex
	published [][]byte
	topics    []string
}

func newMQTTTestServer(t *testing.T, deliver [][]byte, echo bool) *mqttTestServer {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := &mqttTestServer{ln: ln, deliver: deliver, echo: echo}
	go srv.accept()
	t.Cleanup(func() { _ = ln.Close() })
	return srv
}

func (s *mqttTestServer) accept() {
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			return
		}
		go s.handle(conn)
	}
}

func mqttTestString(value string) []byte {
	out := make([]byte, 2)
	binary.BigEndian.PutUint16(out, uint16(len(value)))
	return append(out, value...)
}

func mqttTestReadString(body []byte, pos *int) (string, error) {
	if *pos+2 > len(body) {
		return "", io.ErrUnexpectedEOF
	}
	length := int(binary.BigEndian.Uint16(body[*pos:]))
	*pos += 2
	if *pos+length > len(body) {
		return "", io.ErrUnexpectedEOF
	}
	value := string(body[*pos : *pos+length])
	*pos += length
	return value, nil
}

func mqttTestRead(conn net.Conn) (byte, []byte, error) {
	var head [1]byte
	if _, err := io.ReadFull(conn, head[:]); err != nil {
		return 0, nil, err
	}
	length := 0
	multiplier := 1
	for i := 0; i < 4; i++ {
		var b [1]byte
		if _, err := io.ReadFull(conn, b[:]); err != nil {
			return 0, nil, err
		}
		length += int(b[0]&0x7f) * multiplier
		if b[0]&0x80 == 0 {
			break
		}
		multiplier *= 128
	}
	body := make([]byte, length)
	if _, err := io.ReadFull(conn, body); err != nil {
		return 0, nil, err
	}
	return head[0], body, nil
}

func mqttTestWrite(conn net.Conn, header byte, body []byte) error {
	packet := []byte{header}
	length := len(body)
	for {
		digit := byte(length % 128)
		length /= 128
		if length > 0 {
			digit |= 0x80
		}
		packet = append(packet, digit)
		if length == 0 {
			break
		}
	}
	packet = append(packet, body...)
	_, err := conn.Write(packet)
	return err
}

func (s *mqttTestServer) handle(conn net.Conn) {
	defer conn.Close()
	header, _, err := mqttTestRead(conn)
	if err != nil || header>>4 != 1 {
		return
	}
	if mqttTestWrite(conn, 0x20, []byte{0, 0}) != nil {
		return
	}
	header, body, err := mqttTestRead(conn)
	if err != nil || header>>4 != 8 || len(body) < 3 {
		return
	}
	pos := 2
	topic, err := mqttTestReadString(body, &pos)
	if err != nil {
		return
	}
	packetID := body[0:2]
	if mqttTestWrite(conn, 0x90, append(append([]byte{}, packetID...), 0)) != nil {
		return
	}
	for i, payload := range s.deliver {
		out := mqttTestString(topic)
		qosHeader := byte(0x30)
		if len(payload) > 0 && i%2 == 0 && len(payload) < 200 {
			qosHeader = 0x32
			out = append(out, 0, byte(i+1))
		}
		out = append(out, payload...)
		if mqttTestWrite(conn, qosHeader, out) != nil {
			return
		}
		if qosHeader == 0x32 {
			if _, _, err := mqttTestRead(conn); err != nil { // PUBACK
				return
			}
		}
	}
	for {
		header, body, err := mqttTestRead(conn)
		if err != nil {
			return
		}
		switch header >> 4 {
		case 3:
			qos := (header >> 1) & 3
			pos := 0
			topic, err := mqttTestReadString(body, &pos)
			if err != nil {
				return
			}
			var id []byte
			if qos > 0 {
				if pos+2 > len(body) {
					return
				}
				id = body[pos : pos+2]
				pos += 2
			}
			payload := body[pos:]
			s.mu.Lock()
			s.published = append(s.published, append([]byte{}, payload...))
			s.topics = append(s.topics, topic)
			s.mu.Unlock()
			if len(id) > 0 {
				if mqttTestWrite(conn, 0x40, id) != nil {
					return
				}
			}
			if s.echo {
				if mqttTestWrite(conn, 0x30, append(mqttTestString(topic), payload...)) != nil {
					return
				}
			}
		case 14:
			return
		}
	}
}

func (s *mqttTestServer) received() ([][]byte, []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([][]byte{}, s.published...), append([]string{}, s.topics...)
}

func mqttTestRequest(target, topic string, opts map[string]string, body string) *Request {
	all := map[string]string{"topic": topic}
	for key, value := range opts {
		all[key] = value
	}
	req := &Request{Kind: "mqtt", Target: "mqtt://" + target, Opts: all}
	if body != "" {
		req.Body = &Payload{Encoding: "utf8", Data: body}
	}
	return req
}

func TestMQTTPublishAndReceive(t *testing.T) {
	srv := newMQTTTestServer(t, nil, true)
	req := mqttTestRequest(srv.ln.Addr().String(), "demo/topic", map[string]string{"qos": "1"}, "hello")
	resp, err := Execute(messagingConfig(), req)
	if err != nil {
		t.Fatal(err)
	}
	published, topics := srv.received()
	if len(published) != 1 || string(published[0]) != "hello" || topics[0] != "demo/topic" {
		t.Fatalf("published = %q %v", published, topics)
	}
	data := decodeMessagingJSON(t, resp)
	messages := data["messages"].([]any)
	if len(messages) != 1 {
		t.Fatalf("messages = %#v", messages)
	}
	message := messages[0].(map[string]any)
	if message["topic"] != "demo/topic" || message["qos"] != float64(0) {
		t.Fatalf("message = %#v", message)
	}
	if payload := messagingPayload(message); payload["data"] != "hello" || payload["encoding"] != "utf8" {
		t.Fatalf("payload = %#v", payload)
	}
	if resp.Headers["x-messages"] != "1" || resp.Headers["x-topic"] != "demo/topic" {
		t.Fatalf("headers = %#v", resp.Headers)
	}
}

func TestMQTTSubscribeWindowAndQoSAck(t *testing.T) {
	srv := newMQTTTestServer(t, [][]byte{[]byte("first"), []byte("second")}, false)
	resp, err := Execute(messagingConfig(), mqttTestRequest(srv.ln.Addr().String(), "demo", map[string]string{"messages": "2", "duration_ms": "500"}, ""))
	if err != nil {
		t.Fatal(err)
	}
	messages := decodeMessagingJSON(t, resp)["messages"].([]any)
	if len(messages) != 2 {
		t.Fatalf("messages = %#v", messages)
	}
	if messagingPayload(messages[0].(map[string]any))["data"] != "first" ||
		messagingPayload(messages[1].(map[string]any))["data"] != "second" {
		t.Fatalf("messages = %#v", messages)
	}

	empty := newMQTTTestServer(t, nil, false)
	started := time.Now()
	resp, err = Execute(messagingConfig(), mqttTestRequest(empty.ln.Addr().String(), "empty", map[string]string{"duration_ms": "40"}, ""))
	if err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("empty window took %v", elapsed)
	}
	if messages := decodeMessagingJSON(t, resp)["messages"].([]any); len(messages) != 0 {
		t.Fatalf("empty messages = %#v", messages)
	}
}

func TestMQTTCapsSSRFAndValidation(t *testing.T) {
	srv := newMQTTTestServer(t, [][]byte{[]byte(strings.Repeat("x", 100))}, false)
	cfg := messagingConfig()
	cfg.MaxBytes = 64
	_, err := Execute(cfg, mqttTestRequest(srv.ln.Addr().String(), "big", nil, ""))
	if err == nil || !strings.Contains(err.Error(), "max_bytes") {
		t.Fatalf("cap err = %v", err)
	}

	private := Config{EnableServerSide: true, TimeoutMS: 500}
	_, err = Execute(private, mqttTestRequest(srv.ln.Addr().String(), "demo", nil, ""))
	if err == nil || !strings.Contains(err.Error(), "private") {
		t.Fatalf("ssrf err = %v", err)
	}

	for name, req := range map[string]*Request{
		"missing topic": {Kind: "mqtt", Target: "mqtt://127.0.0.1:1", Opts: map[string]string{}},
		"qos 2":         {Kind: "mqtt", Target: "mqtt://127.0.0.1:1", Opts: map[string]string{"topic": "x", "qos": "2"}},
		"tls":           {Kind: "mqtt", Target: "mqtts://127.0.0.1:1", Opts: map[string]string{"topic": "x"}},
	} {
		if _, err := Execute(messagingConfig(), req); err == nil {
			t.Fatalf("%s was accepted", name)
		}
	}
}

func TestMQTTStreamSession(t *testing.T) {
	srv := newMQTTTestServer(t, [][]byte{[]byte("one"), []byte("two")}, false)
	stream, err := OpenStream(messagingConfig(), mqttTestRequest(srv.ln.Addr().String(), "stream", nil, ""))
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()

	first := make(chan []byte, 1)
	go func() {
		frame, err := stream.ReadFrame()
		if err == nil {
			first <- frame
		}
		close(first)
	}()
	select {
	case frame := <-first:
		if string(frame) != "one" {
			t.Fatalf("first frame = %q", frame)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("first frame timed out")
	}
	frame, err := stream.ReadFrame()
	if err != nil || string(frame) != "two" {
		t.Fatalf("second frame = %q err=%v", frame, err)
	}
	if err := stream.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestMQTTConnectFailure(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		_ = conn.Close()
	}()
	_, err = Execute(messagingConfig(), mqttTestRequest(addr, "demo", nil, ""))
	_ = ln.Close()
	if err == nil || !strings.Contains(err.Error(), "mqtt") {
		t.Fatalf("connect err = %v", err)
	}
}

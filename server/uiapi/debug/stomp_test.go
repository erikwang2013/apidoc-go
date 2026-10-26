//go:build apidoc_debug_stomp || apidoc_debug_messaging || apidoc_debug_all

package debug

import (
	"bufio"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

type stompTestMessage struct {
	destination string
	headers     map[string]string
	body        string
}

type stompTestServer struct {
	addr    string
	deliver []stompTestMessage
	echo    bool

	mu        sync.Mutex
	sent      []stompTestMessage
	connected map[string]string
}

func newSTOMPTestServer(t *testing.T, deliver []stompTestMessage, echo bool) *stompTestServer {
	t.Helper()
	ln := listenLocal(t)
	srv := &stompTestServer{addr: ln.Addr().String(), deliver: deliver, echo: echo}
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

func stompTestWrite(connWriter *bufio.Writer, command string, headers [][2]string, body string) error {
	if _, err := connWriter.WriteString(command + "\n"); err != nil {
		return err
	}
	hasLength := false
	for _, header := range headers {
		if header[0] == "content-length" {
			hasLength = true
		}
		if _, err := connWriter.WriteString(header[0] + ":" + header[1] + "\n"); err != nil {
			return err
		}
	}
	if !hasLength && strings.ContainsRune(body, 0) {
		if _, err := connWriter.WriteString("content-length:" + strconv.Itoa(len(body)) + "\n"); err != nil {
			return err
		}
	}
	if _, err := connWriter.WriteString("\n" + body + "\x00"); err != nil {
		return err
	}
	return connWriter.Flush()
}

func stompTestRead(reader *bufio.Reader) (string, map[string]string, string, error) {
	command := ""
	for command == "" {
		line, err := reader.ReadString('\n')
		if err != nil {
			return "", nil, "", err
		}
		command = strings.TrimRight(line, "\r\n")
	}
	headers := map[string]string{}
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return "", nil, "", err
		}
		text := strings.TrimRight(line, "\r\n")
		if text == "" {
			break
		}
		key, value, _ := strings.Cut(text, ":")
		headers[key] = value
	}
	var body strings.Builder
	if raw, found := headers["content-length"]; found {
		length, err := strconv.Atoi(raw)
		if err != nil {
			return "", nil, "", err
		}
		buf := make([]byte, length)
		if _, err := io.ReadFull(reader, buf); err != nil {
			return "", nil, "", err
		}
		body.Write(buf)
		nul, err := reader.ReadByte()
		if err != nil || nul != 0 {
			return "", nil, "", err
		}
	} else {
		for {
			ch, err := reader.ReadByte()
			if err != nil {
				return "", nil, "", err
			}
			if ch == 0 {
				break
			}
			body.WriteByte(ch)
		}
	}
	return command, headers, body.String(), nil
}

func (s *stompTestServer) handle(conn net.Conn) {
	defer conn.Close()
	reader := bufio.NewReader(conn)
	writer := bufio.NewWriter(conn)
	destination := ""
	for {
		command, headers, body, err := stompTestRead(reader)
		if err != nil {
			return
		}
		switch command {
		case "CONNECT", "STOMP":
			s.mu.Lock()
			s.connected = headers
			s.mu.Unlock()
			if stompTestWrite(writer, "CONNECTED", [][2]string{{"version", "1.2"}, {"heart-beat", "0,0"}}, "") != nil {
				return
			}
		case "SUBSCRIBE":
			destination = headers["destination"]
			for i, message := range s.deliver {
				frameHeaders := [][2]string{
					{"destination", message.destination},
					{"message-id", "m" + strconv.Itoa(i+1)},
					{"subscription", headers["id"]},
				}
				for key, value := range message.headers {
					frameHeaders = append(frameHeaders, [2]string{key, value})
				}
				if stompTestWrite(writer, "MESSAGE", frameHeaders, message.body) != nil {
					return
				}
			}
		case "SEND":
			message := stompTestMessage{destination: headers["destination"], headers: headers, body: body}
			s.mu.Lock()
			s.sent = append(s.sent, message)
			s.mu.Unlock()
			if s.echo {
				if stompTestWrite(writer, "MESSAGE", [][2]string{{"destination", destination}, {"message-id", "echo"}}, body) != nil {
					return
				}
			}
		case "DISCONNECT":
			return
		}
	}
}

func (s *stompTestServer) received() []stompTestMessage {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]stompTestMessage{}, s.sent...)
}

func (s *stompTestServer) connectHeaders() map[string]string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := map[string]string{}
	for key, value := range s.connected {
		out[key] = value
	}
	return out
}

func stompTestRequest(addr, destination string, opts map[string]string, body string) *Request {
	all := map[string]string{"destination": destination}
	for key, value := range opts {
		all[key] = value
	}
	req := &Request{Kind: "stomp", Target: "tcp://" + addr, Opts: all}
	if body != "" {
		req.Body = &Payload{Encoding: "utf8", Data: body}
	}
	return req
}

func TestSTOMPEscapeRoundTrip(t *testing.T) {
	value := "queue/a:b\nline\r\\end"
	escaped := stompEscape(value)
	if escaped != `queue/a\cb\nline\r\\end` {
		t.Fatalf("escaped = %q", escaped)
	}
	if got := stompUnescape(escaped); got != value {
		t.Fatalf("unescaped = %q", got)
	}
	if got := stompUnescape(`\\n`); got != `\n` {
		t.Fatalf("trailing backslash = %q", got)
	}
}

func TestSTOMPSendAndReceive(t *testing.T) {
	srv := newSTOMPTestServer(t, nil, true)
	resp, err := Execute(messagingConfig(), stompTestRequest(srv.addr, "queue/demo", map[string]string{"login": "user", "passcode": "pw"}, "hello"))
	if err != nil {
		t.Fatal(err)
	}
	sent := srv.received()
	if len(sent) != 1 || sent[0].destination != "queue/demo" || sent[0].body != "hello" {
		t.Fatalf("sent = %#v", sent)
	}
	connected := srv.connectHeaders()
	if connected["login"] != "user" || connected["passcode"] != "pw" || connected["accept-version"] != "1.2" {
		t.Fatalf("connect headers = %#v", connected)
	}
	messages := decodeMessagingJSON(t, resp)["messages"].([]any)
	if len(messages) != 1 {
		t.Fatalf("messages = %#v", messages)
	}
	message := messages[0].(map[string]any)
	if message["destination"] != "queue/demo" || messagingPayload(message)["data"] != "hello" {
		t.Fatalf("message = %#v", message)
	}
}

func TestSTOMPHeadersAndContentLength(t *testing.T) {
	srv := newSTOMPTestServer(t, []stompTestMessage{{
		destination: "queue/demo",
		headers:     map[string]string{"x-path": `queue/a\cb`, "x-note": `line\ntwo`},
		body:        "binary\x00body",
	}}, false)
	resp, err := Execute(messagingConfig(), stompTestRequest(srv.addr, "queue/demo", nil, ""))
	if err != nil {
		t.Fatal(err)
	}
	message := decodeMessagingJSON(t, resp)["messages"].([]any)[0].(map[string]any)
	headers := message["headers"].(map[string]any)
	if headers["x-path"] != "queue/a:b" || headers["x-note"] != "line\ntwo" {
		t.Fatalf("headers = %#v", headers)
	}
	if messagingPayload(message)["data"] != "binary\x00body" {
		t.Fatalf("payload = %#v", messagingPayload(message))
	}
}

func TestSTOMPWindowCapsSSRFAndValidation(t *testing.T) {
	empty := newSTOMPTestServer(t, nil, false)
	started := time.Now()
	resp, err := Execute(messagingConfig(), stompTestRequest(empty.addr, "queue/empty", map[string]string{"duration_ms": "40"}, ""))
	if err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("empty window took %v", elapsed)
	}
	if messages := decodeMessagingJSON(t, resp)["messages"].([]any); len(messages) != 0 {
		t.Fatalf("empty messages = %#v", messages)
	}

	big := newSTOMPTestServer(t, []stompTestMessage{{destination: "big", body: strings.Repeat("x", 100)}}, false)
	cfg := messagingConfig()
	cfg.MaxBytes = 64
	_, err = Execute(cfg, stompTestRequest(big.addr, "big", nil, ""))
	if err == nil || !strings.Contains(err.Error(), "max_bytes") {
		t.Fatalf("cap err = %v", err)
	}

	private := Config{EnableServerSide: true, TimeoutMS: 500}
	_, err = Execute(private, stompTestRequest(empty.addr, "queue/x", nil, ""))
	if err == nil || !strings.Contains(err.Error(), "private") {
		t.Fatalf("ssrf err = %v", err)
	}

	for name, req := range map[string]*Request{
		"missing destination": {Kind: "stomp", Target: "tcp://127.0.0.1:1", Opts: map[string]string{}},
		"tls transport":       {Kind: "stomp", Target: "tcp://127.0.0.1:1", Opts: map[string]string{"destination": "q", "transport": "tls"}},
	} {
		if _, err := Execute(messagingConfig(), req); err == nil {
			t.Fatalf("%s was accepted", name)
		}
	}
}

func TestSTOMPStreamSession(t *testing.T) {
	srv := newSTOMPTestServer(t, []stompTestMessage{
		{destination: "queue/stream", body: "one"},
		{destination: "queue/stream", body: "two"},
	}, false)
	stream, err := OpenStream(messagingConfig(), stompTestRequest(srv.addr, "queue/stream", nil, ""))
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

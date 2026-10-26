//go:build apidoc_debug_stomp || apidoc_debug_messaging || apidoc_debug_all

package debug

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"time"
)

func init() {
	RegisterExecutor("stomp", executeSTOMP)
	RegisterStream("stomp", openSTOMPStream)
}

// stompFrame is one STOMP 1.2 frame.
type stompFrame struct {
	command string
	headers map[string]string
	body    []byte
}

type stompClient struct {
	conn     net.Conn
	reader   *stompReader
	host     string
	limit    int
	passcode string
}

type stompReader struct {
	conn  net.Conn
	buf   []byte
	limit int
}

func (r *stompReader) fill() error {
	chunk := make([]byte, 4096)
	n, err := r.conn.Read(chunk)
	if n > 0 {
		r.buf = append(r.buf, chunk[:n]...)
	}
	if err != nil {
		return err
	}
	if n == 0 {
		return io.EOF
	}
	return nil
}

func (r *stompReader) readLine() ([]byte, error) {
	for {
		if pos := bytes.IndexByte(r.buf, '\n'); pos >= 0 {
			line := append([]byte{}, r.buf[:pos+1]...)
			r.buf = r.buf[pos+1:]
			return line, nil
		}
		if len(r.buf) > r.limit {
			return nil, fmt.Errorf("stomp frame header exceeds max_bytes (%d)", r.limit)
		}
		if err := r.fill(); err != nil {
			return nil, err
		}
	}
}

func (r *stompReader) readExact(n int) ([]byte, error) {
	for len(r.buf) < n {
		if err := r.fill(); err != nil {
			return nil, err
		}
	}
	out := append([]byte{}, r.buf[:n]...)
	r.buf = r.buf[n:]
	return out, nil
}

func (r *stompReader) readUntilNUL(limit int) ([]byte, error) {
	for {
		if pos := bytes.IndexByte(r.buf, 0); pos >= 0 {
			body := append([]byte{}, r.buf[:pos]...)
			r.buf = r.buf[pos+1:]
			return body, nil
		}
		if len(r.buf) > limit {
			return nil, fmt.Errorf("stomp frame body exceeds max_bytes (%d)", limit)
		}
		if err := r.fill(); err != nil {
			return nil, err
		}
	}
}

func (r *stompReader) readFrame() (*stompFrame, error) {
	var command string
	for {
		line, err := r.readLine()
		if err != nil {
			return nil, err
		}
		command = strings.TrimRight(string(line), "\r\n")
		if command != "" {
			break
		}
	}
	headers := map[string]string{}
	for {
		line, err := r.readLine()
		if err != nil {
			return nil, err
		}
		text := strings.TrimRight(string(line), "\r\n")
		if text == "" {
			break
		}
		key, value, found := strings.Cut(text, ":")
		if !found {
			continue
		}
		headers[stompUnescape(key)] = stompUnescape(value)
	}
	var body []byte
	var err error
	if raw, found := headers["content-length"]; found {
		length, parseErr := strconv.Atoi(raw)
		if parseErr != nil || length < 0 {
			return nil, fmt.Errorf("invalid stomp content-length `%s`", raw)
		}
		if length > r.limit {
			return nil, fmt.Errorf("stomp frame body exceeds max_bytes (%d)", r.limit)
		}
		if body, err = r.readExact(length); err != nil {
			return nil, err
		}
		nul, err := r.readExact(1)
		if err != nil {
			return nil, err
		}
		if nul[0] != 0 {
			return nil, fmt.Errorf("stomp frame missing NUL terminator")
		}
	} else if body, err = r.readUntilNUL(r.limit); err != nil {
		return nil, err
	}
	return &stompFrame{command: command, headers: headers, body: body}, nil
}

func stompEscape(value string) string {
	var out strings.Builder
	for _, ch := range value {
		switch ch {
		case '\\':
			out.WriteString("\\\\")
		case '\r':
			out.WriteString("\\r")
		case '\n':
			out.WriteString("\\n")
		case ':':
			out.WriteString("\\c")
		default:
			out.WriteRune(ch)
		}
	}
	return out.String()
}

func stompUnescape(value string) string {
	var out strings.Builder
	escaped := false
	for _, ch := range value {
		if !escaped {
			if ch == '\\' {
				escaped = true
			} else {
				out.WriteRune(ch)
			}
			continue
		}
		escaped = false
		switch ch {
		case 'n':
			out.WriteByte('\n')
		case 'r':
			out.WriteByte('\r')
		case 'c':
			out.WriteByte(':')
		default:
			out.WriteRune(ch)
		}
	}
	if escaped {
		out.WriteByte('\\')
	}
	return out.String()
}

func (c *stompClient) writeFrame(command string, headers [][2]string, body []byte) error {
	var out strings.Builder
	out.WriteString(command)
	out.WriteByte('\n')
	for _, header := range headers {
		out.WriteString(stompEscape(header[0]))
		out.WriteByte(':')
		out.WriteString(stompEscape(header[1]))
		out.WriteByte('\n')
	}
	if body != nil {
		out.WriteString("content-length:")
		out.WriteString(strconv.Itoa(len(body)))
		out.WriteByte('\n')
	}
	out.WriteByte('\n')
	packet := []byte(out.String())
	packet = append(packet, body...)
	packet = append(packet, 0)
	_, err := c.conn.Write(packet)
	return err
}

func stompDestination(req *Request) (string, error) {
	destination := req.Opts["destination"]
	if destination == "" {
		return "", fmt.Errorf("stomp requires opts.destination")
	}
	return destination, nil
}

func stompConnect(cfg Config, req *Request) (*stompClient, error) {
	if transport := req.Opts["transport"]; transport != "" && transport != "tcp" {
		return nil, fmt.Errorf("stomp transport `%s` is not supported by the server executor; use tcp", transport)
	}
	host, port, _, err := parseMessagingTarget(req.Target, "tcp", "61613", map[string]bool{
		"tcp": true, "stomp": true,
	})
	if err != nil {
		return nil, err
	}
	if _, err := stompDestination(req); err != nil {
		return nil, err
	}
	passcode := req.Opts["passcode"]
	conn, err := dialMessaging(cfg, host, port, req.Timeout(cfg))
	if err != nil {
		return nil, redactedError(fmt.Sprintf("stomp %v", err), passcode)
	}
	client := &stompClient{
		conn:     conn,
		reader:   &stompReader{conn: conn, limit: cfg.Limit()},
		host:     host,
		limit:    cfg.Limit(),
		passcode: passcode,
	}
	_ = conn.SetDeadline(time.Now().Add(req.Timeout(cfg)))
	if err := client.handshake(req); err != nil {
		_ = conn.Close()
		return nil, err
	}
	_ = conn.SetDeadline(time.Time{})
	return client, nil
}

func (c *stompClient) handshake(req *Request) error {
	passcode := req.Opts["passcode"]
	headers := [][2]string{
		{"accept-version", "1.2"},
		{"host", c.host},
		{"heart-beat", "0,0"},
	}
	if login := req.Opts["login"]; login != "" {
		headers = append(headers, [2]string{"login", login})
	}
	if _, found := req.Opts["passcode"]; found {
		headers = append(headers, [2]string{"passcode", passcode})
	}
	if err := c.writeFrame("CONNECT", headers, nil); err != nil {
		return redactedError(fmt.Sprintf("stomp connect failed: %v", err), passcode)
	}
	for {
		frame, err := c.reader.readFrame()
		if err != nil {
			return redactedError(fmt.Sprintf("stomp connect failed: %v", err), passcode)
		}
		switch frame.command {
		case "CONNECTED":
			destination, err := stompDestination(req)
			if err != nil {
				return err
			}
			subscribe := [][2]string{
				{"id", "0"},
				{"destination", destination},
				{"ack", "auto"},
			}
			if err := c.writeFrame("SUBSCRIBE", subscribe, nil); err != nil {
				return redactedError(fmt.Sprintf("stomp subscribe failed: %v", err), passcode)
			}
			return c.publish(req)
		case "ERROR":
			return redactedError(fmt.Sprintf("stomp server error: %s", stompErrorText(frame)), passcode)
		}
	}
}

func (c *stompClient) publish(req *Request) error {
	body, err := req.Body.Decode()
	if err != nil {
		return err
	}
	if len(body) == 0 {
		return nil
	}
	if len(body) > c.limit {
		return fmt.Errorf("request body exceeds max_bytes (%d)", c.limit)
	}
	destination, err := stompDestination(req)
	if err != nil {
		return err
	}
	headers := [][2]string{{"destination", destination}}
	for name, value := range req.Headers {
		if strings.EqualFold(name, "content-length") {
			continue
		}
		headers = append(headers, [2]string{name, value})
	}
	if writeErr := c.writeFrame("SEND", headers, body); writeErr != nil {
		return redactedError(fmt.Sprintf("stomp send failed: %v", writeErr), req.Opts["passcode"])
	}
	return nil
}

func stompErrorText(frame *stompFrame) string {
	message := frame.headers["message"]
	body := strings.TrimSpace(string(frame.body))
	switch {
	case message == "" && body == "":
		return "unknown error"
	case message == "":
		return body
	case body == "":
		return message
	default:
		return message + ": " + body
	}
}

func (c *stompClient) message(frame *stompFrame) (map[string]any, error) {
	headers := map[string]string{}
	for name, value := range frame.headers {
		headers[name] = value
	}
	return map[string]any{
		"destination": frame.headers["destination"],
		"headers":     headers,
		"payload":     payloadJSON(FromBytes(frame.body)),
	}, nil
}

func executeSTOMP(cfg Config, req *Request) (*Response, error) {
	started := time.Now()
	client, err := stompConnect(cfg, req)
	if err != nil {
		return nil, err
	}
	defer client.close()
	want := wantMessages(req)
	deadline := time.Now().Add(windowDuration(cfg, req))
	messages := []map[string]any{}
	total := 0
	for len(messages) < want && time.Now().Before(deadline) {
		_ = client.conn.SetReadDeadline(deadline)
		frame, err := client.reader.readFrame()
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				break
			}
			return nil, redactedError(fmt.Sprintf("stomp receive failed: %v", err), req.Opts["passcode"])
		}
		switch frame.command {
		case "MESSAGE":
			message, err := client.message(frame)
			if err != nil {
				return nil, err
			}
			if err := addMessageBytes(&total, len(frame.body), cfg.Limit()); err != nil {
				return nil, err
			}
			messages = append(messages, message)
		case "ERROR":
			return nil, redactedError(fmt.Sprintf("stomp server error: %s", stompErrorText(frame)), req.Opts["passcode"])
		}
	}
	encoded, err := json.Marshal(map[string]any{"messages": messages})
	if err != nil {
		return nil, fmt.Errorf("stomp response could not be encoded: %v", err)
	}
	return &Response{
		Status:  0,
		MS:      time.Since(started).Milliseconds(),
		Headers: map[string]string{"x-messages": strconv.Itoa(len(messages))},
		Body:    Payload{Encoding: "utf8", Data: string(encoded)},
	}, nil
}

func (c *stompClient) close() {
	_ = c.conn.SetWriteDeadline(time.Now().Add(100 * time.Millisecond))
	_ = c.writeFrame("DISCONNECT", nil, nil)
	_ = c.conn.Close()
}

type stompStream struct {
	client *stompClient
}

func openSTOMPStream(cfg Config, req *Request) (StreamSession, error) {
	client, err := stompConnect(cfg, req)
	if err != nil {
		return nil, err
	}
	return &stompStream{client: client}, nil
}

// ReadFrame returns one MESSAGE body; ERROR frames end the session.
func (s *stompStream) ReadFrame() ([]byte, error) {
	for {
		frame, err := s.client.reader.readFrame()
		if err != nil {
			return nil, err
		}
		switch frame.command {
		case "MESSAGE":
			return append([]byte{}, frame.body...), nil
		case "ERROR":
			return nil, redactErr(fmt.Errorf("stomp server error: %s", stompErrorText(frame)), s.client.passcode)
		}
	}
}

func (s *stompStream) Close() error {
	s.client.close()
	return nil
}

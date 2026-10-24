// Package socketio adapts Socket.IO endpoints using a minimal Engine.IO v4
// client over WebSocket. URLs use socketio:// or socketios://; the URL path
// is the Socket.IO namespace and query parameters are passed to the
// handshake. The Engine.IO endpoint defaults to /socket.io/ and can be
// overridden with the engine_path option.
package socketio

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	ws "github.com/gorilla/websocket"

	"github.com/erikwang2013/apidoc-go"
	"github.com/erikwang2013/apidoc-go/example/debugger-web/internal/opt"
)

// Protocol is the apidoc protocol key.
const Protocol = "socketio"

// New returns a streaming debugger for Socket.IO endpoints.
func New() apidoc.StreamDebugger { return debugger{} }

type debugger struct{}

func (debugger) Protocol() string { return Protocol }

func (debugger) Spec() apidoc.DebugSpec {
	return apidoc.DebugSpec{
		Mode:         "server",
		Streaming:    true,
		Actions:      []string{"CONNECT", "SEND", "EMIT", "SUBSCRIBE", "GET"},
		Encodings:    []string{"json", "text"},
		BodyIn:       "payload",
		ResponseView: "transcript",
		ConnectFields: []apidoc.DebugField{
			{Name: "engine_path", Label: "Engine.IO path", Default: "/socket.io/"},
			{Name: "auth", Label: "Auth JSON", Placeholder: `{"token":"..."}`},
			{Name: "timeout_ms", Label: "Connect timeout (ms)", Type: "number", Default: "10000"},
		},
	}
}

// Open performs the Engine.IO handshake and joins the namespace.
func (debugger) Open(ctx context.Context, req apidoc.DebugRequest) (apidoc.Session, error) {
	endpoint, ns, err := endpointURL(req.URL, req.Options["engine_path"])
	if err != nil {
		return nil, err
	}
	h := http.Header{}
	for k, v := range req.Headers {
		h.Set(k, v)
	}
	d := ws.Dialer{HandshakeTimeout: 5 * time.Second}
	conn, _, err := d.DialContext(ctx, endpoint, h)
	if err != nil {
		return nil, fmt.Errorf("socketio: %w", err)
	}
	s := newSession(conn, ns)
	if err := s.handshake(ctx, req); err != nil {
		_ = conn.Close()
		return nil, err
	}
	go s.read()
	return s, nil
}

// Exec opens a session, sends Body, and returns the first event.
func (d debugger) Exec(ctx context.Context, req apidoc.DebugRequest) (*apidoc.DebugResponse, error) {
	start := time.Now()
	s, err := d.Open(ctx, req)
	if err != nil {
		return nil, err
	}
	defer s.Close()

	if len(req.Body) > 0 {
		if err := s.Send(ctx, req.Body, map[string]string{"encoding": req.Encoding}); err != nil {
			return nil, err
		}
	}
	timer := time.NewTimer(opt.Duration(req.Options, "timeout_ms", 5*time.Second, 60*time.Second))
	defer timer.Stop()
	var evs []apidoc.DebugEvent
	for {
		select {
		case ev, ok := <-s.Events():
			if !ok {
				return ioResponse("CLOSED", evs, start), nil
			}
			evs = append(evs, ev)
			if ev.Name == "handshake" || ev.Name == "connect" {
				continue
			}
			if ev.Name == "error" {
				return ioResponse("ERROR", evs, start), nil
			}
			return ioResponse("OK", evs, start), nil
		case <-timer.C:
			return ioResponse("TIMEOUT", evs, start), nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

func ioResponse(status string, evs []apidoc.DebugEvent, start time.Time) *apidoc.DebugResponse {
	r := &apidoc.DebugResponse{Status: status, Events: evs, Duration: time.Since(start)}
	for i := len(evs) - 1; i >= 0; i-- {
		if evs[i].Name != "handshake" && evs[i].Name != "connect" {
			r.Body = []byte(evs[i].Data)
			r.Encoding = evs[i].Encoding
			break
		}
	}
	return r
}

func endpointURL(raw, enginePath string) (string, string, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "", "", fmt.Errorf("socketio: bad url %q", raw)
	}
	scheme := ""
	switch u.Scheme {
	case "ws", "wss":
		scheme = u.Scheme
	case "socketio":
		scheme = "ws"
	case "socketios":
		scheme = "wss"
	default:
		return "", "", fmt.Errorf("socketio: bad scheme %q", u.Scheme)
	}
	ns := u.Path
	if ns == "" {
		ns = "/"
	}
	if !strings.HasPrefix(ns, "/") {
		ns = "/" + ns
	}
	if enginePath == "" {
		enginePath = "/socket.io/"
	}
	if !strings.HasPrefix(enginePath, "/") {
		enginePath = "/" + enginePath
	}
	q := u.Query()
	q.Set("EIO", "4")
	q.Set("transport", "websocket")
	ep := url.URL{Scheme: scheme, Host: u.Host, Path: enginePath, RawQuery: q.Encode()}
	return ep.String(), ns, nil
}

type session struct {
	conn      *ws.Conn
	ns        string
	events    chan apidoc.DebugEvent
	done      chan struct{}
	readDone  chan struct{}
	closeOnce sync.Once
	writeMu   sync.Mutex
}

func newSession(conn *ws.Conn, ns string) *session {
	conn.SetReadLimit(4 << 20)
	return &session{
		conn:     conn,
		ns:       ns,
		events:   make(chan apidoc.DebugEvent, 256),
		done:     make(chan struct{}),
		readDone: make(chan struct{}),
	}
}

func (s *session) Events() <-chan apidoc.DebugEvent { return s.events }

func (s *session) nsPrefix() string {
	if s.ns == "/" {
		return ""
	}
	return s.ns + ","
}

func (s *session) handshake(ctx context.Context, req apidoc.DebugRequest) error {
	timeout := opt.Duration(req.Options, "timeout_ms", 10*time.Second, 30*time.Second)
	deadline := time.Now().Add(timeout)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	_ = s.conn.SetReadDeadline(deadline)
	stop := context.AfterFunc(ctx, func() { _ = s.conn.SetReadDeadline(time.Now()) })
	defer stop()
	defer func() { _ = s.conn.SetReadDeadline(time.Time{}) }()

	pkt, err := s.readPacket()
	if err != nil {
		return fmt.Errorf("socketio: handshake: %w", err)
	}
	if len(pkt) == 0 || pkt[0] != '0' {
		return fmt.Errorf("socketio: unexpected handshake %q", pkt)
	}
	s.emit(apidoc.DebugEvent{Time: time.Now(), Direction: "in", Name: "handshake", Data: pkt[1:], Encoding: "json"})

	connect := "40" + s.nsPrefix()
	if auth := req.Options["auth"]; auth != "" {
		if !json.Valid([]byte(auth)) {
			return errors.New("socketio: auth option must be JSON")
		}
		connect += auth
	}
	if err := s.write(connect); err != nil {
		return fmt.Errorf("socketio: connect: %w", err)
	}
	ackPrefix := "40" + s.nsPrefix()
	for {
		pkt, err := s.readPacket()
		if err != nil {
			return fmt.Errorf("socketio: connect: %w", err)
		}
		switch {
		case pkt == "2":
			if err := s.write("3"); err != nil {
				return fmt.Errorf("socketio: pong: %w", err)
			}
		case strings.HasPrefix(pkt, ackPrefix):
			data := s.stripNamespace(pkt[2:])
			s.emit(apidoc.DebugEvent{Time: time.Now(), Direction: "in", Name: "connect", Data: data, Encoding: "json"})
			return nil
		case strings.HasPrefix(pkt, "44"):
			return fmt.Errorf("socketio: connect refused: %s", s.stripNamespace(pkt[2:]))
		}
	}
}

func (s *session) Send(ctx context.Context, msg []byte, meta map[string]string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	pkt, err := s.packet(msg, meta)
	if err != nil {
		return err
	}
	deadline := time.Now().Add(10 * time.Second)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	stop := context.AfterFunc(ctx, func() { _ = s.conn.SetWriteDeadline(time.Now()) })
	defer stop()

	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	_ = s.conn.SetWriteDeadline(deadline)
	return s.conn.WriteMessage(ws.TextMessage, []byte(pkt))
}

// packet builds a Socket.IO event packet. A JSON array is used verbatim as
// the event arguments; anything else becomes the payload of meta["event"]
// (default "message"). meta["raw"] sends the bytes as a raw Engine.IO packet.
func (s *session) packet(msg []byte, meta map[string]string) (string, error) {
	if meta["raw"] == "true" {
		return string(msg), nil
	}
	prefix := "42" + s.nsPrefix()
	trimmed := bytes.TrimSpace(msg)
	if len(trimmed) > 0 && trimmed[0] == '[' && json.Valid(trimmed) {
		return prefix + string(trimmed), nil
	}
	if len(trimmed) == 0 {
		b, _ := json.Marshal([]string{eventName(meta)})
		return prefix + string(b), nil
	}
	var v any
	if err := json.Unmarshal(trimmed, &v); err != nil {
		v = string(msg)
	}
	b, err := json.Marshal([]any{eventName(meta), v})
	if err != nil {
		return "", fmt.Errorf("socketio: %w", err)
	}
	return prefix + string(b), nil
}

func eventName(meta map[string]string) string {
	if meta["event"] != "" {
		return meta["event"]
	}
	return "message"
}

func (s *session) Close() error {
	s.closeOnce.Do(func() {
		close(s.done)
		_ = s.write("41" + s.nsPrefix())
		_ = s.conn.Close()
		<-s.readDone
	})
	return nil
}

func (s *session) read() {
	defer close(s.readDone)
	defer close(s.events)
	for {
		mt, data, err := s.conn.ReadMessage()
		if err != nil {
			s.emit(apidoc.DebugEvent{Time: time.Now(), Direction: "in", Name: "error", Data: err.Error()})
			return
		}
		if mt != ws.TextMessage {
			s.emit(apidoc.DebugEvent{
				Time: time.Now(), Direction: "in", Name: "binary",
				Data: base64.StdEncoding.EncodeToString(data), Encoding: "base64",
			})
			continue
		}
		pkt := string(data)
		if pkt == "" {
			continue
		}
		switch pkt[0] {
		case '2':
			if err := s.write("3"); err != nil {
				s.emit(apidoc.DebugEvent{Time: time.Now(), Direction: "in", Name: "error", Data: err.Error()})
				return
			}
			continue
		case '3', '6':
			continue
		case '1':
			s.emit(apidoc.DebugEvent{Time: time.Now(), Direction: "in", Name: "close", Data: "engine.io close"})
			return
		}
		switch {
		case strings.HasPrefix(pkt, "42"):
			body := stripID(s.stripNamespace(pkt[2:]))
			s.emit(apidoc.DebugEvent{
				Time: time.Now(), Direction: "in", Name: eventFromArgs(body), Data: body, Encoding: "json",
			})
		case strings.HasPrefix(pkt, "43"):
			body := stripID(s.stripNamespace(pkt[2:]))
			s.emit(apidoc.DebugEvent{Time: time.Now(), Direction: "in", Name: "ack", Data: body, Encoding: "json"})
		case strings.HasPrefix(pkt, "44"):
			body := s.stripNamespace(pkt[2:])
			s.emit(apidoc.DebugEvent{Time: time.Now(), Direction: "in", Name: "error", Data: body, Encoding: "json"})
		case strings.HasPrefix(pkt, "40"):
			s.emit(apidoc.DebugEvent{
				Time: time.Now(), Direction: "in", Name: "connect",
				Data: s.stripNamespace(pkt[2:]), Encoding: "json",
			})
		case strings.HasPrefix(pkt, "41"):
			s.emit(apidoc.DebugEvent{
				Time: time.Now(), Direction: "in", Name: "disconnect",
				Data: s.stripNamespace(pkt[2:]),
			})
		default:
			s.emit(apidoc.DebugEvent{Time: time.Now(), Direction: "in", Name: "packet", Data: pkt})
		}
	}
}

// stripNamespace drops a leading "/namespace," from a Socket.IO packet body.
func (s *session) stripNamespace(body string) string {
	if !strings.HasPrefix(body, "/") {
		return body
	}
	if i := strings.IndexByte(body, ','); i >= 0 {
		return body[i+1:]
	}
	return body
}

// stripID drops a leading acknowledgment id from a Socket.IO packet body.
func stripID(body string) string {
	i := 0
	for i < len(body) && body[i] >= '0' && body[i] <= '9' {
		i++
	}
	return body[i:]
}

func eventFromArgs(body string) string {
	var args []json.RawMessage
	if json.Unmarshal([]byte(body), &args) != nil || len(args) == 0 {
		return "event"
	}
	var name string
	if json.Unmarshal(args[0], &name) != nil || name == "" {
		return "event"
	}
	return name
}

func (s *session) readPacket() (string, error) {
	mt, data, err := s.conn.ReadMessage()
	if err != nil {
		return "", err
	}
	if mt != ws.TextMessage {
		return "", fmt.Errorf("unexpected binary frame")
	}
	return string(data), nil
}

func (s *session) write(msg string) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	_ = s.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	return s.conn.WriteMessage(ws.TextMessage, []byte(msg))
}

func (s *session) emit(ev apidoc.DebugEvent) {
	select {
	case s.events <- ev:
	case <-s.done:
	}
}

// Package websocket adapts WebSocket endpoints (RFC 6455) to apidoc's
// streaming debug interface. URLs use ws:// or wss://.
package websocket

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"time"

	ws "github.com/gorilla/websocket"

	"github.com/erikwang2013/apidoc-go"
	"github.com/erikwang2013/apidoc-go/example/debugger-web/internal/opt"
)

// Protocol is the apidoc protocol key.
const Protocol = "ws"

// New returns a streaming debugger for ws:// and wss:// endpoints.
func New() apidoc.StreamDebugger { return debugger{} }

type debugger struct{}

func (debugger) Protocol() string { return Protocol }

func (debugger) Spec() apidoc.DebugSpec {
	return apidoc.DebugSpec{
		Mode:         "server",
		Streaming:    true,
		Actions:      []string{"CONNECT", "SEND", "SUBSCRIBE", "GET"},
		Encodings:    []string{"text", "json", "hex", "base64"},
		BodyIn:       "payload",
		ResponseView: "transcript",
		ConnectFields: []apidoc.DebugField{
			{Name: "subprotocol", Label: "Subprotocol", Placeholder: "chat"},
			{Name: "read_timeout_ms", Label: "One-shot read timeout (ms)", Type: "number", Default: "5000"},
		},
	}
}

// Open dials the endpoint and starts forwarding incoming frames.
func (d debugger) Open(ctx context.Context, req apidoc.DebugRequest) (apidoc.Session, error) {
	conn, _, err := d.dial(ctx, req)
	if err != nil {
		return nil, err
	}
	s := newSession(conn)
	go s.read()
	return s, nil
}

// Exec connects, optionally sends Body, and returns the first reply.
func (d debugger) Exec(ctx context.Context, req apidoc.DebugRequest) (*apidoc.DebugResponse, error) {
	start := time.Now()
	conn, resp, err := d.dial(ctx, req)
	if err != nil {
		return nil, err
	}
	s := newSession(conn)
	go s.read()
	defer s.Close()

	headers := map[string]string{}
	if sp := conn.Subprotocol(); sp != "" {
		headers["subprotocol"] = sp
	}
	if resp != nil {
		headers["http_status"] = resp.Status
	}
	if len(req.Body) > 0 {
		meta := map[string]string{"encoding": req.Encoding}
		if req.Encoding == "hex" || req.Encoding == "base64" {
			meta["type"] = "binary"
		}
		if err := s.Send(ctx, req.Body, meta); err != nil {
			return nil, err
		}
	}

	timer := time.NewTimer(opt.Duration(req.Options, "read_timeout_ms", 5*time.Second, 60*time.Second))
	defer timer.Stop()
	var evs []apidoc.DebugEvent
	for {
		select {
		case ev, ok := <-s.events:
			if !ok {
				return wsResponse("CLOSED", evs, headers, start), nil
			}
			evs = append(evs, ev)
			if ev.Name != "close" && ev.Name != "error" {
				return wsResponse("OK", evs, headers, start), nil
			}
		case <-timer.C:
			return wsResponse("TIMEOUT", evs, headers, start), nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

func wsResponse(status string, evs []apidoc.DebugEvent, headers map[string]string, start time.Time) *apidoc.DebugResponse {
	r := &apidoc.DebugResponse{Status: status, Headers: headers, Events: evs, Duration: time.Since(start)}
	if len(evs) > 0 {
		r.Body = []byte(evs[0].Data)
		r.Encoding = evs[0].Encoding
	}
	return r
}

func (debugger) dial(ctx context.Context, req apidoc.DebugRequest) (*ws.Conn, *http.Response, error) {
	u, err := url.Parse(req.URL)
	if err != nil || u.Host == "" || (u.Scheme != "ws" && u.Scheme != "wss") {
		return nil, nil, fmt.Errorf("ws: bad url %q", req.URL)
	}
	h := http.Header{}
	for k, v := range req.Headers {
		h.Set(k, v)
	}
	d := ws.Dialer{
		HandshakeTimeout: 5 * time.Second,
		Subprotocols:     opt.List(req.Options["subprotocol"]),
	}
	if u.Scheme == "wss" {
		d.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	}
	conn, resp, err := d.DialContext(ctx, u.String(), h)
	if err != nil {
		return nil, resp, fmt.Errorf("ws: %w", err)
	}
	return conn, resp, nil
}

type session struct {
	conn      *ws.Conn
	events    chan apidoc.DebugEvent
	done      chan struct{}
	readDone  chan struct{}
	closeOnce sync.Once
	writeMu   sync.Mutex
}

func newSession(conn *ws.Conn) *session {
	conn.SetReadLimit(4 << 20)
	return &session{
		conn:     conn,
		events:   make(chan apidoc.DebugEvent, 256),
		done:     make(chan struct{}),
		readDone: make(chan struct{}),
	}
}

func (s *session) Events() <-chan apidoc.DebugEvent { return s.events }

func (s *session) Send(ctx context.Context, msg []byte, meta map[string]string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	mt := ws.TextMessage
	if meta["type"] == "binary" || meta["encoding"] == "hex" || meta["encoding"] == "base64" {
		mt = ws.BinaryMessage
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
	return s.conn.WriteMessage(mt, msg)
}

func (s *session) Close() error {
	s.closeOnce.Do(func() {
		close(s.done)
		msg := ws.FormatCloseMessage(ws.CloseNormalClosure, "")
		_ = s.conn.WriteControl(ws.CloseMessage, msg, time.Now().Add(time.Second))
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
			ev := apidoc.DebugEvent{Time: time.Now(), Direction: "in", Name: "error", Data: err.Error()}
			if ce, ok := err.(*ws.CloseError); ok {
				ev.Name = "close"
				if ce.Text == "" {
					ev.Data = http.StatusText(ce.Code)
				} else {
					ev.Data = ce.Text
				}
			}
			s.emit(ev)
			return
		}
		ev := apidoc.DebugEvent{Time: time.Now(), Direction: "in", Name: "message"}
		if mt == ws.BinaryMessage {
			ev.Data = base64.StdEncoding.EncodeToString(data)
			ev.Encoding = "base64"
		} else {
			ev.Data = string(data)
			ev.Encoding = "text"
			if json.Valid(data) {
				ev.Encoding = "json"
			}
		}
		if !s.emit(ev) {
			return
		}
	}
}

func (s *session) emit(ev apidoc.DebugEvent) bool {
	select {
	case s.events <- ev:
		return true
	case <-s.done:
		return false
	}
}

// Package sse adapts server-sent event streams to apidoc's streaming debug
// interface. URLs use sse:// or sses://; plain http(s) URLs are accepted too.
package sse

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/erikwang2013/apidoc-go"
	"github.com/erikwang2013/apidoc-go/example/debugger-web/internal/httpc"
	"github.com/erikwang2013/apidoc-go/example/debugger-web/internal/opt"
	sseio "github.com/erikwang2013/apidoc-go/example/debugger-web/internal/sse"
)

// Protocol is the apidoc protocol key.
const Protocol = "sse"

// New returns a streaming debugger for server-sent event endpoints.
func New() apidoc.StreamDebugger { return debugger{} }

type debugger struct{}

func (debugger) Protocol() string { return Protocol }

func (debugger) Spec() apidoc.DebugSpec {
	return apidoc.DebugSpec{
		Mode:         "server",
		Streaming:    true,
		Actions:      []string{"SUBSCRIBE", "GET"},
		Encodings:    []string{"text", "json"},
		BodyIn:       "payload",
		ResponseView: "transcript",
		ConnectFields: []apidoc.DebugField{
			{Name: "last_event_id", Label: "Last-Event-ID", Placeholder: "resume from event id"},
			{Name: "read_timeout_ms", Label: "One-shot read timeout (ms)", Type: "number", Default: "3000"},
		},
	}
}

// Open subscribes to the stream and forwards every dispatched event.
func (debugger) Open(ctx context.Context, req apidoc.DebugRequest) (apidoc.Session, error) {
	target, err := streamURL(req.URL)
	if err != nil {
		return nil, err
	}
	// The session outlives the open call, and callers commonly cancel ctx
	// when Open returns. Honor ctx until the response headers arrive, then
	// detach the body so only Close (or the caller's session manager) ends it.
	streamCtx, cancel := context.WithCancel(context.Background())
	stop := context.AfterFunc(ctx, cancel)
	hreq, err := http.NewRequestWithContext(streamCtx, http.MethodGet, target, nil)
	if err != nil {
		stop()
		cancel()
		return nil, fmt.Errorf("sse: %w", err)
	}
	hreq.Header.Set("Accept", "text/event-stream")
	hreq.Header.Set("Cache-Control", "no-cache")
	for k, v := range req.Headers {
		hreq.Header.Set(k, v)
	}
	if id := req.Options["last_event_id"]; id != "" {
		hreq.Header.Set("Last-Event-ID", id)
	}
	resp, err := httpc.Client.Do(hreq)
	if err != nil {
		stop()
		cancel()
		return nil, fmt.Errorf("sse: %w", err)
	}
	if !stop() {
		_ = resp.Body.Close()
		cancel()
		return nil, fmt.Errorf("sse: connection open canceled: %w", ctx.Err())
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		_ = resp.Body.Close()
		cancel()
		return nil, fmt.Errorf("sse: %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	s := &session{
		events:   make(chan apidoc.DebugEvent, 256),
		done:     make(chan struct{}),
		body:     resp.Body,
		readDone: make(chan struct{}),
		cancel:   cancel,
	}
	go s.read()
	return s, nil
}

// Exec subscribes and returns the first event, for one-shot probes.
func (d debugger) Exec(ctx context.Context, req apidoc.DebugRequest) (*apidoc.DebugResponse, error) {
	start := time.Now()
	s, err := d.Open(ctx, req)
	if err != nil {
		return nil, err
	}
	defer s.Close()

	timer := time.NewTimer(opt.Duration(req.Options, "read_timeout_ms", 3*time.Second, 60*time.Second))
	defer timer.Stop()
	select {
	case ev, ok := <-s.Events():
		if !ok {
			return &apidoc.DebugResponse{Status: "CLOSED", Duration: time.Since(start)}, nil
		}
		status := "OK"
		if ev.Name == "error" {
			status = "ERROR"
		}
		return &apidoc.DebugResponse{
			Status:   status,
			Body:     []byte(ev.Data),
			Encoding: ev.Encoding,
			Events:   []apidoc.DebugEvent{ev},
			Duration: time.Since(start),
		}, nil
	case <-timer.C:
		return &apidoc.DebugResponse{Status: "TIMEOUT", Duration: time.Since(start)}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func streamURL(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "", fmt.Errorf("sse: bad url %q", raw)
	}
	switch u.Scheme {
	case "http", "https":
	case "sse":
		u.Scheme = "http"
	case "sses":
		u.Scheme = "https"
	default:
		return "", fmt.Errorf("sse: bad scheme %q", u.Scheme)
	}
	return u.String(), nil
}

type session struct {
	events    chan apidoc.DebugEvent
	done      chan struct{}
	body      io.ReadCloser
	readDone  chan struct{}
	cancel    context.CancelFunc
	closeOnce sync.Once
}

func (s *session) Events() <-chan apidoc.DebugEvent { return s.events }

// Send reports that server-sent events carry no client-to-server channel.
func (s *session) Send(ctx context.Context, _ []byte, _ map[string]string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return errors.New("sse: server-sent events are receive-only")
}

func (s *session) Close() error {
	s.closeOnce.Do(func() {
		close(s.done)
		s.cancel()
		_ = s.body.Close()
		<-s.readDone
	})
	return nil
}

func (s *session) read() {
	defer close(s.readDone)
	defer close(s.events)
	err := sseio.Read(s.body, func(ev sseio.Event) error {
		name := ev.Name
		if name == "" {
			name = "message"
		}
		enc := "text"
		if json.Valid([]byte(ev.Data)) {
			enc = "json"
		}
		s.emit(apidoc.DebugEvent{Time: time.Now(), Direction: "in", Name: name, Data: ev.Data, Encoding: enc})
		return nil
	})
	if err != nil {
		s.emit(apidoc.DebugEvent{Time: time.Now(), Direction: "in", Name: "error", Data: err.Error()})
	}
}

func (s *session) emit(ev apidoc.DebugEvent) {
	select {
	case s.events <- ev:
	case <-s.done:
	}
}

// Package stomp implements an apidoc StreamDebugger for STOMP 1.2.
package stomp

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/url"
	"strings"
	"sync"
	"time"

	stompgo "github.com/go-stomp/stomp/v3"
	"github.com/go-stomp/stomp/v3/frame"

	"github.com/erikwang2013/apidoc-go"
	"github.com/erikwang2013/apidoc-go/example/debugger-messaging/internal/msg"
)

// New returns a STOMP debugger. URL form: stomp://host:61613/queue/name
// (stomps:// enables TLS); the URL path is the STOMP destination.
func New() apidoc.StreamDebugger { return debugger{} }

var _ apidoc.StreamDebugger = debugger{}

type debugger struct{}

func (debugger) Protocol() string { return "stomp" }

func (debugger) Spec() apidoc.DebugSpec {
	return apidoc.DebugSpec{
		Mode:         "server",
		Actions:      []string{"SEND", "SUBSCRIBE"},
		Encodings:    msg.Encodings,
		BodyIn:       "payload",
		ResponseView: "transcript",
		Streaming:    true,
		ConnectFields: []apidoc.DebugField{
			{Name: "login", Label: "Login", Type: "text"},
			{Name: "passcode", Label: "Passcode", Type: "password", Secret: true},
			{Name: "ack", Label: "Ack mode", Type: "select", Default: "auto",
				Options: []string{"auto", "client", "client-individual"}},
		},
	}
}

// Exec sends one message and disconnects.
func (debugger) Exec(ctx context.Context, req apidoc.DebugRequest) (*apidoc.DebugResponse, error) {
	if !strings.EqualFold(req.Method, "SEND") {
		return nil, fmt.Errorf("stomp: %s needs a streaming session (subscribe/consume); use SEND here", req.Method)
	}
	start := time.Now()
	u, destination, err := target(req.URL)
	if err != nil {
		return nil, err
	}
	payload, err := msg.Payload(req.Body, req.Encoding)
	if err != nil {
		return nil, err
	}
	conn, err := dial(ctx, u, req.Options)
	if err != nil {
		return nil, err
	}
	defer conn.Disconnect()

	if err := conn.Send(destination, contentType(req.Headers), payload, sendOpts(req.Headers)...); err != nil {
		return nil, fmt.Errorf("stomp: send: %w", err)
	}
	return &apidoc.DebugResponse{
		Status:   "SENT",
		Headers:  map[string]string{"destination": destination},
		Duration: time.Since(start),
	}, nil
}

// Open subscribes to the destination and streams messages until Close.
func (debugger) Open(ctx context.Context, req apidoc.DebugRequest) (apidoc.Session, error) {
	u, destination, err := target(req.URL)
	if err != nil {
		return nil, err
	}
	ack, err := parseAck(req.Options)
	if err != nil {
		return nil, err
	}
	conn, err := dial(ctx, u, req.Options)
	if err != nil {
		return nil, err
	}
	// Receipt makes Open wait until the broker created the subscription.
	sub, err := conn.Subscribe(destination, ack, stompgo.SubscribeOpt.Receipt(""))
	if err != nil {
		conn.Disconnect()
		return nil, fmt.Errorf("stomp: subscribe %q: %w", destination, err)
	}
	sctx, cancel := context.WithCancel(context.Background())
	s := &session{
		conn: conn, sub: sub, destination: destination,
		events: msg.NewEventQueue(), cancel: cancel, done: make(chan struct{}),
	}
	go s.read(sctx)
	return s, nil
}

type session struct {
	conn        *stompgo.Conn
	sub         *stompgo.Subscription
	destination string
	events      *msg.EventQueue
	cancel      context.CancelFunc
	done        chan struct{}
	once        sync.Once
}

func (s *session) read(ctx context.Context) {
	defer close(s.done)
	defer s.events.Close()
	for {
		select {
		case <-ctx.Done():
			return
		case m, ok := <-s.sub.C:
			if !ok {
				return
			}
			if m.Err != nil {
				s.events.Push(msg.Err(m.Err))
				continue
			}
			s.events.Push(msg.In(m.Destination, m.Body))
			if m.ShouldAck() {
				_ = s.conn.Ack(m)
			}
		}
	}
}

func (s *session) Send(_ context.Context, payload []byte, meta map[string]string) error {
	payload, err := msg.Payload(payload, meta["encoding"])
	if err != nil {
		return err
	}
	if err := s.conn.Send(s.destination, contentType(meta), payload, sendOpts(meta)...); err != nil {
		return fmt.Errorf("stomp: send: %w", err)
	}
	s.events.Push(msg.Out(s.destination, payload))
	return nil
}

func (s *session) Events() <-chan apidoc.DebugEvent { return s.events.Ch() }

func (s *session) Close() error {
	s.once.Do(func() {
		s.cancel()
		_ = s.sub.Unsubscribe()
		_ = s.conn.Disconnect()
		<-s.done
	})
	return nil
}

func target(raw string) (*url.URL, string, error) {
	u, err := msg.URL(raw, "stomp", "stomp", "stomps")
	if err != nil {
		return nil, "", err
	}
	if len(u.Path) < 2 || !strings.HasPrefix(u.Path, "/") {
		return nil, "", fmt.Errorf("stomp: url %q has no destination path", raw)
	}
	return u, u.Path, nil
}

func dial(ctx context.Context, u *url.URL, opts map[string]string) (*stompgo.Conn, error) {
	defaultPort := "61613"
	if u.Scheme == "stomps" {
		defaultPort = "61614"
	}
	dialer := net.Dialer{Timeout: msg.WaitTimeout(ctx, 5*time.Second)}
	conn, err := dialer.DialContext(ctx, "tcp", msg.Addr(u, defaultPort))
	if err != nil {
		return nil, fmt.Errorf("stomp: dial: %w", err)
	}
	if u.Scheme == "stomps" {
		tlsConn := tls.Client(conn, &tls.Config{ServerName: u.Hostname()})
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			conn.Close()
			return nil, fmt.Errorf("stomp: tls: %w", err)
		}
		conn = tlsConn
	}
	wait := msg.WaitTimeout(ctx, 10*time.Second)
	sc, err := stompgo.ConnectWithContext(ctx, conn,
		stompgo.ConnOpt.Login(msg.Option(opts, "login", ""), msg.Option(opts, "passcode", "")),
		stompgo.ConnOpt.Host(u.Hostname()),
		stompgo.ConnOpt.MsgSendTimeout(wait),
		stompgo.ConnOpt.RcvReceiptTimeout(wait),
		stompgo.ConnOpt.DisconnectReceiptTimeout(wait),
		stompgo.ConnOpt.SubscribeReceiptTimeout(wait),
	)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("stomp: connect: %w", err)
	}
	return sc, nil
}

func parseAck(opts map[string]string) (stompgo.AckMode, error) {
	switch strings.ToLower(msg.Option(opts, "ack", "auto")) {
	case "auto":
		return stompgo.AckAuto, nil
	case "client":
		return stompgo.AckClient, nil
	case "client-individual":
		return stompgo.AckClientIndividual, nil
	default:
		return 0, fmt.Errorf("stomp: invalid ack mode %q", msg.Option(opts, "ack", "auto"))
	}
}

func contentType(values map[string]string) string {
	return msg.Option(values, "content-type", "text/plain")
}

// sendOpts asks for a RECEIPT so Send returns only after the broker has the
// frame; other headers are forwarded as message headers.
func sendOpts(values map[string]string) []func(*frame.Frame) error {
	opts := []func(*frame.Frame) error{stompgo.SendOpt.Receipt}
	for k, v := range values {
		if k == "content-type" || k == "encoding" {
			continue
		}
		opts = append(opts, stompgo.SendOpt.Header(k, v))
	}
	return opts
}

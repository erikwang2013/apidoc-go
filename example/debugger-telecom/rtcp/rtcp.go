// Package rtcp is an apidoc streaming debug adapter for RTCP over UDP.
// Compound packets are validated with pion/rtcp and shown as a hex transcript
// with one summary event per control packet.
package rtcp

import (
	"context"
	"encoding/hex"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"sync"
	"time"

	pionrtcp "github.com/pion/rtcp"

	"github.com/erikwang2013/apidoc-go"
)

const maxPacket = 65535

type debugger struct{}

// New returns the RTCP debugger.
func New() apidoc.StreamDebugger { return debugger{} }

func (debugger) Protocol() string { return "rtcp" }

func (debugger) Spec() apidoc.DebugSpec {
	return apidoc.DebugSpec{
		Mode:         "server",
		Actions:      []string{"SEND"},
		Encodings:    []string{"hex", "base64"},
		ResponseView: "transcript",
		Streaming:    true,
		ConnectFields: []apidoc.DebugField{
			{Name: "timeout_ms", Label: "Reply wait (ms, exec)", Type: "number", Default: "500"},
			{Name: "raw", Label: "Send without RTCP validation", Type: "bool"},
		},
	}
}

func (d debugger) Exec(ctx context.Context, req apidoc.DebugRequest) (*apidoc.DebugResponse, error) {
	sess, err := d.Open(ctx, req)
	if err != nil {
		return nil, err
	}
	defer sess.Close()
	start := time.Now()
	if len(req.Body) > 0 {
		if err := sess.Send(ctx, req.Body, req.Options); err != nil {
			return nil, err
		}
	}
	timeout := 500 * time.Millisecond
	if ms, err := strconv.Atoi(req.Options["timeout_ms"]); err == nil && ms >= 0 {
		timeout = time.Duration(ms) * time.Millisecond
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	res := &apidoc.DebugResponse{Status: "SENT"}
	for {
		select {
		case ev, ok := <-sess.Events():
			if !ok {
				res.Duration = time.Since(start)
				return res, nil
			}
			res.Events = append(res.Events, ev)
			if ev.Direction == "in" {
				res.Status = "OK"
				res.Duration = time.Since(start)
				return res, nil
			}
		case <-timer.C:
			res.Duration = time.Since(start)
			return res, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

func (d debugger) Open(ctx context.Context, req apidoc.DebugRequest) (apidoc.Session, error) {
	target, err := parseURL(req.URL)
	if err != nil {
		return nil, err
	}
	conn, err := (&net.Dialer{}).DialContext(ctx, "udp", target)
	if err != nil {
		return nil, fmt.Errorf("rtcp: dial %s: %w", target, err)
	}
	s := &session{conn: conn, events: make(chan apidoc.DebugEvent, 256), done: make(chan struct{})}
	go s.readLoop()
	return s, nil
}

type session struct {
	conn   net.Conn
	events chan apidoc.DebugEvent
	done   chan struct{}
	once   sync.Once
}

func (s *session) readLoop() {
	defer close(s.done)
	defer close(s.events)
	buf := make([]byte, maxPacket)
	for {
		n, err := s.conn.Read(buf)
		if err != nil {
			return
		}
		for _, ev := range packetEvents("in", buf[:n]) {
			select {
			case s.events <- ev:
			default:
			}
		}
	}
}

func (s *session) Send(ctx context.Context, msg []byte, meta map[string]string) error {
	if len(msg) == 0 {
		return fmt.Errorf("rtcp: empty packet")
	}
	if raw := meta["raw"]; raw != "1" && raw != "true" {
		if _, err := pionrtcp.Unmarshal(msg); err != nil {
			return fmt.Errorf("rtcp: invalid packet: %w", err)
		}
	}
	ev := apidoc.DebugEvent{
		Time: time.Now(), Direction: "out", Name: "RTCP len=" + strconv.Itoa(len(msg)),
		Data: hex.EncodeToString(msg), Encoding: "hex",
	}
	select {
	case s.events <- ev:
	default:
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	_, err := s.conn.Write(msg)
	return err
}

func (s *session) Events() <-chan apidoc.DebugEvent { return s.events }

func (s *session) Close() error {
	err := s.conn.Close()
	s.once.Do(func() { <-s.done })
	return err
}

func parseURL(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("rtcp: bad url: %w", err)
	}
	if u.Scheme != "rtcp" && u.Scheme != "udp" {
		return "", fmt.Errorf("rtcp: unsupported scheme %q", u.Scheme)
	}
	if u.Host == "" {
		return "", fmt.Errorf("rtcp: url has no host")
	}
	return u.Host, nil
}

func packetEvents(dir string, buf []byte) []apidoc.DebugEvent {
	pkts, err := pionrtcp.Unmarshal(buf)
	if err != nil {
		return []apidoc.DebugEvent{{
			Time: time.Now(), Direction: dir, Name: "RTCP invalid: " + err.Error(),
			Data: hex.EncodeToString(buf), Encoding: "hex",
		}}
	}
	events := make([]apidoc.DebugEvent, 0, len(pkts))
	for _, p := range pkts {
		events = append(events, apidoc.DebugEvent{
			Time: time.Now(), Direction: dir, Name: fmt.Sprintf("RTCP %T", p),
			Data: hex.EncodeToString(buf), Encoding: "hex",
		})
	}
	return events
}

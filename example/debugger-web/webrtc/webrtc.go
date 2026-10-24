// Package webrtc adapts WebRTC data channels to apidoc's streaming debug
// interface. URLs use webrtc://host/path: the path is an HTTP signaling
// endpoint that accepts an SDP offer (POST, application/sdp) and returns an
// SDP answer. This keeps the adapter stateless; production signaling (rooms,
// tokens, trickle ICE) is the host application's concern.
package webrtc

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	pion "github.com/pion/webrtc/v4"

	"github.com/erikwang2013/apidoc-go"
	"github.com/erikwang2013/apidoc-go/example/debugger-web/internal/httpc"
	"github.com/erikwang2013/apidoc-go/example/debugger-web/internal/opt"
)

// Protocol is the apidoc protocol key.
const Protocol = "webrtc"

// New returns a streaming debugger for WebRTC data channels.
func New() apidoc.StreamDebugger { return debugger{} }

type debugger struct{}

func (debugger) Protocol() string { return Protocol }

func (debugger) Spec() apidoc.DebugSpec {
	return apidoc.DebugSpec{
		Mode:         "server",
		Streaming:    true,
		Actions:      []string{"CONNECT", "SEND"},
		Encodings:    []string{"text", "json", "hex", "base64"},
		BodyIn:       "payload",
		ResponseView: "transcript",
		ConnectFields: []apidoc.DebugField{
			{Name: "label", Label: "DataChannel label", Default: "data"},
			{Name: "stun", Label: "STUN servers", Placeholder: "stun:stun.example.com:3478"},
			{Name: "timeout_ms", Label: "Connect timeout (ms)", Type: "number", Default: "15000"},
			{Name: "read_timeout_ms", Label: "One-shot read timeout (ms)", Type: "number", Default: "10000"},
		},
	}
}

// Open negotiates a data channel through the signaling endpoint.
func (debugger) Open(ctx context.Context, req apidoc.DebugRequest) (apidoc.Session, error) {
	signal, err := signalURL(req.URL)
	if err != nil {
		return nil, err
	}
	timeout := opt.Duration(req.Options, "timeout_ms", 15*time.Second, 60*time.Second)

	cfg := pion.Configuration{}
	if stun := req.Options["stun"]; stun != "" {
		cfg.ICEServers = []pion.ICEServer{{URLs: opt.List(stun)}}
	}
	pc, err := pion.NewPeerConnection(cfg)
	if err != nil {
		return nil, fmt.Errorf("webrtc: %w", err)
	}
	label := req.Options["label"]
	if label == "" {
		label = "data"
	}
	dc, err := pc.CreateDataChannel(label, nil)
	if err != nil {
		_ = pc.Close()
		return nil, fmt.Errorf("webrtc: %w", err)
	}
	s := newSession(pc, dc)

	offer, err := pc.CreateOffer(nil)
	if err != nil {
		s.Close()
		return nil, fmt.Errorf("webrtc: %w", err)
	}
	if err := pc.SetLocalDescription(offer); err != nil {
		s.Close()
		return nil, fmt.Errorf("webrtc: %w", err)
	}
	select {
	case <-pion.GatheringCompletePromise(pc):
	case <-time.After(timeout):
		s.Close()
		return nil, errors.New("webrtc: ICE gathering timed out")
	case <-ctx.Done():
		s.Close()
		return nil, ctx.Err()
	}
	answer, err := exchange(ctx, signal, pc.LocalDescription().SDP, req.Headers)
	if err != nil {
		s.Close()
		return nil, err
	}
	if err := pc.SetRemoteDescription(pion.SessionDescription{Type: pion.SDPTypeAnswer, SDP: answer}); err != nil {
		s.Close()
		return nil, fmt.Errorf("webrtc: %w", err)
	}
	select {
	case <-s.opened:
		return s, nil
	case <-time.After(timeout):
		s.Close()
		return nil, errors.New("webrtc: data channel did not open")
	case <-ctx.Done():
		s.Close()
		return nil, ctx.Err()
	}
}

// Exec opens a channel, sends Body, and returns the first reply.
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
	timer := time.NewTimer(opt.Duration(req.Options, "read_timeout_ms", 10*time.Second, 60*time.Second))
	defer timer.Stop()
	var evs []apidoc.DebugEvent
	for {
		select {
		case ev, ok := <-s.Events():
			if !ok {
				return rtcResponse("CLOSED", evs, start), nil
			}
			evs = append(evs, ev)
			if ev.Name == "close" || ev.Name == "error" {
				continue
			}
			return rtcResponse("OK", evs, start), nil
		case <-timer.C:
			return rtcResponse("TIMEOUT", evs, start), nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

func rtcResponse(status string, evs []apidoc.DebugEvent, start time.Time) *apidoc.DebugResponse {
	r := &apidoc.DebugResponse{Status: status, Events: evs, Duration: time.Since(start)}
	if len(evs) > 0 {
		r.Body = []byte(evs[0].Data)
		r.Encoding = evs[0].Encoding
	}
	return r
}

func signalURL(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "", fmt.Errorf("webrtc: bad url %q", raw)
	}
	switch u.Scheme {
	case "http", "https":
	case "webrtc":
		u.Scheme = "http"
	case "webrtcs":
		u.Scheme = "https"
	default:
		return "", fmt.Errorf("webrtc: bad scheme %q", u.Scheme)
	}
	return u.String(), nil
}

func exchange(ctx context.Context, signal, offer string, headers map[string]string) (string, error) {
	hreq, err := http.NewRequestWithContext(ctx, http.MethodPost, signal, strings.NewReader(offer))
	if err != nil {
		return "", fmt.Errorf("webrtc: signaling: %w", err)
	}
	hreq.Header.Set("Content-Type", "application/sdp")
	hreq.Header.Set("Accept", "application/sdp, application/json")
	for k, v := range headers {
		hreq.Header.Set(k, v)
	}
	resp, err := httpc.Client.Do(hreq)
	if err != nil {
		return "", fmt.Errorf("webrtc: signaling: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("webrtc: signaling: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		msg := strings.TrimSpace(string(body))
		if len(msg) > 512 {
			msg = msg[:512]
		}
		return "", fmt.Errorf("webrtc: signaling: %s: %s", resp.Status, msg)
	}
	sdp := strings.TrimSpace(string(body))
	if mt, _, err := mime.ParseMediaType(resp.Header.Get("Content-Type")); err == nil && mt == "application/json" {
		var a struct {
			SDP    string `json:"sdp"`
			Answer string `json:"answer"`
		}
		if err := json.Unmarshal(body, &a); err != nil {
			return "", fmt.Errorf("webrtc: signaling answer: %w", err)
		}
		if a.SDP != "" {
			sdp = a.SDP
		} else {
			sdp = a.Answer
		}
	}
	sdp = strings.TrimSpace(sdp)
	if sdp == "" {
		return "", errors.New("webrtc: empty signaling answer")
	}
	// pion's SDP parser requires a trailing line break, which some
	// signaling servers omit when they write LocalDescription().SDP.
	return sdp + "\r\n", nil
}

type session struct {
	pc       *pion.PeerConnection
	dc       *pion.DataChannel
	events   chan apidoc.DebugEvent
	in       chan apidoc.DebugEvent
	done     chan struct{}
	pumpDone chan struct{}
	opened   chan struct{}
	openOnce sync.Once
	stopOnce sync.Once
}

func newSession(pc *pion.PeerConnection, dc *pion.DataChannel) *session {
	s := &session{
		pc:       pc,
		dc:       dc,
		events:   make(chan apidoc.DebugEvent, 256),
		in:       make(chan apidoc.DebugEvent, 256),
		done:     make(chan struct{}),
		pumpDone: make(chan struct{}),
		opened:   make(chan struct{}),
	}
	dc.OnOpen(func() { s.openOnce.Do(func() { close(s.opened) }) })
	dc.OnMessage(func(m pion.DataChannelMessage) {
		ev := apidoc.DebugEvent{Time: time.Now(), Direction: "in", Name: "message"}
		if m.IsString {
			ev.Data = string(m.Data)
			ev.Encoding = "text"
			if json.Valid(m.Data) {
				ev.Encoding = "json"
			}
		} else {
			ev.Data = base64.StdEncoding.EncodeToString(m.Data)
			ev.Encoding = "base64"
		}
		s.emit(ev)
	})
	dc.OnClose(func() {
		s.emit(apidoc.DebugEvent{Time: time.Now(), Direction: "in", Name: "close", Data: "data channel closed"})
		s.stop()
	})
	pc.OnConnectionStateChange(func(st pion.PeerConnectionState) {
		switch st {
		case pion.PeerConnectionStateFailed:
			s.emit(apidoc.DebugEvent{Time: time.Now(), Direction: "in", Name: "error", Data: "connection failed"})
			s.stop()
		case pion.PeerConnectionStateClosed:
			s.stop()
		}
	})
	go s.pump()
	return s
}

func (s *session) Events() <-chan apidoc.DebugEvent { return s.events }

func (s *session) Send(ctx context.Context, msg []byte, meta map[string]string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if s.dc.ReadyState() != pion.DataChannelStateOpen {
		return errors.New("webrtc: data channel is not open")
	}
	if meta["type"] == "binary" || meta["encoding"] == "hex" || meta["encoding"] == "base64" {
		return s.dc.Send(msg)
	}
	return s.dc.SendText(string(msg))
}

func (s *session) Close() error {
	s.stop()
	_ = s.dc.Close()
	_ = s.pc.Close()
	<-s.pumpDone
	return nil
}

func (s *session) stop() {
	s.stopOnce.Do(func() { close(s.done) })
}

func (s *session) emit(ev apidoc.DebugEvent) {
	select {
	case s.in <- ev:
	case <-s.done:
	}
}

func (s *session) pump() {
	defer close(s.pumpDone)
	defer close(s.events)
	for {
		select {
		case ev := <-s.in:
			select {
			case s.events <- ev:
			case <-s.done:
				return
			}
		case <-s.done:
			return
		}
	}
}

// Package rtsp is an apidoc debug adapter for the RTSP signalling protocol.
// Exec performs OPTIONS / DESCRIBE / PLAY against a server; Open keeps a
// session playing and streams the RTSP, RTP and RTCP transcript as events.
// Sessions can send RTCP (or RTP back-channel) packets with Send.
package rtsp

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/bluenviron/gortsplib/v4"
	"github.com/bluenviron/gortsplib/v4/pkg/base"
	"github.com/bluenviron/gortsplib/v4/pkg/description"
	"github.com/bluenviron/gortsplib/v4/pkg/format"
	"github.com/pion/rtcp"
	"github.com/pion/rtp"

	"github.com/erikwang2013/apidoc-go"
)

type debugger struct{}

// New returns the RTSP debugger.
func New() apidoc.StreamDebugger { return debugger{} }

func (debugger) Protocol() string { return "rtsp" }

func (debugger) Spec() apidoc.DebugSpec {
	return apidoc.DebugSpec{
		Mode:         "server",
		Actions:      []string{"OPTIONS", "DESCRIBE", "PLAY"},
		Encodings:    []string{"text"},
		ResponseView: "transcript",
		Streaming:    true,
		ConnectFields: []apidoc.DebugField{
			{Name: "transport", Label: "RTP transport", Type: "select", Options: []string{"tcp", "udp"}, Default: "tcp"},
			{Name: "timeout_ms", Label: "Read/write timeout (ms)", Type: "number", Default: "5000"},
		},
	}
}

func (debugger) Exec(ctx context.Context, req apidoc.DebugRequest) (*apidoc.DebugResponse, error) {
	method := strings.ToUpper(req.Method)
	if method == "" {
		method = "DESCRIBE"
	}
	switch method {
	case "OPTIONS", "DESCRIBE", "PLAY":
	default:
		return nil, fmt.Errorf("rtsp: unsupported method %q", req.Method)
	}

	var mu sync.Mutex
	var events []apidoc.DebugEvent
	emit := func(ev apidoc.DebugEvent) {
		mu.Lock()
		events = append(events, ev)
		mu.Unlock()
	}
	snapshot := func() []apidoc.DebugEvent {
		mu.Lock()
		defer mu.Unlock()
		return append([]apidoc.DebugEvent(nil), events...)
	}

	start := time.Now()
	c, u, err := startClient(ctx, req, emit)
	if err != nil {
		return nil, err
	}
	defer c.Close()

	res := &apidoc.DebugResponse{Status: "OK", Encoding: "text"}
	var body []byte
	switch method {
	case "OPTIONS":
		resp, err := c.Options(u)
		if err != nil {
			return nil, err
		}
		res.Status = statusOf(resp)
		body = resp.Body
	case "DESCRIBE":
		desc, resp, err := c.Describe(u)
		if err != nil {
			return nil, err
		}
		res.Status = statusOf(resp)
		body, err = desc.Marshal(false)
		if err != nil {
			return nil, err
		}
	case "PLAY":
		desc, resp, err := c.Describe(u)
		if err != nil {
			return nil, err
		}
		res.Status = statusOf(resp)
		if err := c.SetupAll(desc.BaseURL, desc.Medias); err != nil {
			return nil, err
		}
		c.OnPacketRTPAny(func(medi *description.Media, forma format.Format, pkt *rtp.Packet) {
			emit(rtpEvent("in", medi, forma, pkt))
		})
		c.OnPacketRTCPAny(func(medi *description.Media, pkt rtcp.Packet) {
			emit(rtcpEvent("in", medi, pkt))
		})
		playResp, err := c.Play(nil)
		if err != nil {
			return nil, err
		}
		res.Status = statusOf(playResp)
		wait := optionDuration(req.Options, "reply_ms", time.Second)
		select {
		case <-time.After(wait):
		case <-ctx.Done():
		}
	}
	res.Body = body
	res.Events = snapshot()
	res.Duration = time.Since(start)
	return res, nil
}

func (debugger) Open(ctx context.Context, req apidoc.DebugRequest) (apidoc.Session, error) {
	s := &session{
		events: make(chan apidoc.DebugEvent, 256),
		done:   make(chan struct{}),
	}
	s.emit = func(ev apidoc.DebugEvent) {
		select {
		case s.events <- ev:
		default:
		}
	}
	c, u, err := startClient(ctx, req, s.emit)
	if err != nil {
		return nil, err
	}
	s.client = c

	desc, _, err := c.Describe(u)
	if err != nil {
		c.Close()
		return nil, err
	}
	if err := c.SetupAll(desc.BaseURL, desc.Medias); err != nil {
		c.Close()
		return nil, err
	}
	s.desc = desc
	c.OnPacketRTPAny(func(medi *description.Media, forma format.Format, pkt *rtp.Packet) {
		s.emit(rtpEvent("in", medi, forma, pkt))
	})
	c.OnPacketRTCPAny(func(medi *description.Media, pkt rtcp.Packet) {
		s.emit(rtcpEvent("in", medi, pkt))
	})
	if _, err := c.Play(nil); err != nil {
		c.Close()
		return nil, err
	}
	go func() {
		defer close(s.done)
		defer close(s.events)
		_ = c.Wait()
	}()
	return s, nil
}

type session struct {
	client *gortsplib.Client
	desc   *description.Session
	emit   func(apidoc.DebugEvent)

	events chan apidoc.DebugEvent
	done   chan struct{}
	once   sync.Once
}

func (s *session) Send(ctx context.Context, msg []byte, meta map[string]string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(msg) == 0 {
		return fmt.Errorf("rtsp: empty packet")
	}
	if s.desc == nil || len(s.desc.Medias) == 0 {
		return fmt.Errorf("rtsp: session has no media")
	}
	idx := 0
	if v := meta["track"]; v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 || n >= len(s.desc.Medias) {
			return fmt.Errorf("rtsp: track %q out of range", v)
		}
		idx = n
	}
	media := s.desc.Medias[idx]

	if strings.EqualFold(meta["kind"], "rtp") {
		p := &rtp.Packet{}
		if err := p.Unmarshal(msg); err != nil {
			return fmt.Errorf("rtsp: invalid RTP: %w", err)
		}
		s.emit(rtpEvent("out", media, nil, p))
		return s.client.WritePacketRTP(media, p)
	}
	pkts, err := rtcp.Unmarshal(msg)
	if err != nil {
		return fmt.Errorf("rtsp: invalid RTCP: %w", err)
	}
	for _, p := range pkts {
		s.emit(rtcpEvent("out", media, p))
		if err := s.client.WritePacketRTCP(media, p); err != nil {
			return err
		}
	}
	return nil
}

func (s *session) Events() <-chan apidoc.DebugEvent { return s.events }

func (s *session) Close() error {
	s.client.Close()
	s.once.Do(func() { <-s.done })
	return nil
}

// startClient connects and installs transcript callbacks. The client's dialer
// is bound to ctx so setup aborts when the caller gives up.
func startClient(ctx context.Context, req apidoc.DebugRequest, emit func(apidoc.DebugEvent)) (*gortsplib.Client, *base.URL, error) {
	u, err := base.ParseURL(req.URL)
	if err != nil {
		return nil, nil, fmt.Errorf("rtsp: bad url: %w", err)
	}
	if u.Scheme != "rtsp" && u.Scheme != "rtsps" {
		return nil, nil, fmt.Errorf("rtsp: unsupported scheme %q", u.Scheme)
	}
	timeout := optionDuration(req.Options, "timeout_ms", 5*time.Second)
	c := &gortsplib.Client{
		Scheme:       u.Scheme,
		Host:         u.Host,
		ReadTimeout:  timeout,
		WriteTimeout: timeout,
	}
	transport := gortsplib.TransportTCP
	if strings.EqualFold(req.Options["transport"], "udp") {
		transport = gortsplib.TransportUDP
	}
	c.Transport = &transport
	dialer := &net.Dialer{}
	c.DialContext = func(_ context.Context, network, address string) (net.Conn, error) {
		return dialer.DialContext(ctx, network, address)
	}
	c.OnRequest = func(r *base.Request) {
		emit(messageEvent("out", r.String()))
	}
	c.OnResponse = func(r *base.Response) {
		emit(messageEvent("in", r.String()))
	}
	if err := c.Start2(); err != nil {
		c.Close()
		return nil, nil, err
	}
	return c, u, nil
}

func messageEvent(dir, data string) apidoc.DebugEvent {
	name := "RTSP"
	if i := strings.IndexByte(data, ' '); i > 0 {
		name += " " + data[:i]
	}
	return apidoc.DebugEvent{Time: time.Now(), Direction: dir, Name: name, Data: data, Encoding: "text"}
}

func rtpEvent(dir string, medi *description.Media, forma format.Format, pkt *rtp.Packet) apidoc.DebugEvent {
	codec := ""
	if forma != nil {
		codec = forma.Codec()
	}
	return apidoc.DebugEvent{
		Time: time.Now(), Direction: dir,
		Name: fmt.Sprintf("RTP %s seq=%d ts=%d ssrc=%#x pt=%d len=%d",
			codec, pkt.SequenceNumber, pkt.Timestamp, pkt.SSRC, pkt.PayloadType, len(pkt.Payload)),
		Data:     fmt.Sprintf("%v", pkt),
		Encoding: "text",
	}
}

func rtcpEvent(dir string, medi *description.Media, pkt rtcp.Packet) apidoc.DebugEvent {
	return apidoc.DebugEvent{
		Time: time.Now(), Direction: dir,
		Name: fmt.Sprintf("RTCP %T", pkt),
		Data: fmt.Sprintf("%v", pkt), Encoding: "text",
	}
}

func statusOf(res *base.Response) string {
	if res == nil {
		return ""
	}
	return fmt.Sprintf("%d %s", res.StatusCode, res.StatusMessage)
}

func optionDuration(opts map[string]string, key string, def time.Duration) time.Duration {
	if v := opts[key]; v != "" {
		if ms, err := strconv.Atoi(v); err == nil && ms > 0 {
			return time.Duration(ms) * time.Millisecond
		}
	}
	return def
}

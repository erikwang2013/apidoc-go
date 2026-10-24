// Package rtmp is an apidoc streaming debug adapter for RTMP publishing.
// Open connects, creates a stream and publishes; Send writes audio or video
// payloads to the published stream. The transcript records the handshake
// steps and every payload written.
package rtmp

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	yrtmp "github.com/yutopp/go-rtmp"
	"github.com/yutopp/go-rtmp/message"

	"github.com/erikwang2013/apidoc-go"
)

const (
	audioChunkStream = 4
	videoChunkStream = 6
)

type debugger struct{}

// New returns the RTMP debugger.
func New() apidoc.StreamDebugger { return debugger{} }

func (debugger) Protocol() string { return "rtmp" }

func (debugger) Spec() apidoc.DebugSpec {
	return apidoc.DebugSpec{
		Mode:         "server",
		Actions:      []string{"PUBLISH"},
		Encodings:    []string{"hex", "base64"},
		ResponseView: "transcript",
		Streaming:    true,
		ConnectFields: []apidoc.DebugField{
			{Name: "stream", Label: "Publish name (default: URL path)", Type: "text"},
			{Name: "chunk_size", Label: "Chunk size", Type: "number", Default: "4096"},
			{Name: "timeout_ms", Label: "Dial timeout (ms)", Type: "number", Default: "5000"},
		},
	}
}

func (d debugger) Exec(ctx context.Context, req apidoc.DebugRequest) (*apidoc.DebugResponse, error) {
	sess, err := d.Open(ctx, req)
	if err != nil {
		return nil, err
	}
	start := time.Now()
	if len(req.Body) > 0 {
		meta := map[string]string{"type": req.Options["type"]}
		if err := sess.Send(ctx, req.Body, meta); err != nil {
			_ = sess.Close()
			return nil, err
		}
	}
	_ = sess.Close()
	res := &apidoc.DebugResponse{Status: "PUBLISHED", Encoding: "text", Duration: time.Since(start)}
	for ev := range sess.Events() {
		res.Events = append(res.Events, ev)
	}
	return res, nil
}

func (debugger) Open(ctx context.Context, req apidoc.DebugRequest) (apidoc.Session, error) {
	u, err := url.Parse(req.URL)
	if err != nil {
		return nil, fmt.Errorf("rtmp: bad url: %w", err)
	}
	if u.Scheme != "rtmp" && u.Scheme != "rtmps" {
		return nil, fmt.Errorf("rtmp: unsupported scheme %q", u.Scheme)
	}
	if u.Host == "" {
		return nil, fmt.Errorf("rtmp: url has no host")
	}
	name := strings.TrimPrefix(u.Path, "/")
	if v := req.Options["stream"]; v != "" {
		name = v
	}
	if name == "" {
		name = "apidoc"
	}

	timeout := 5 * time.Second
	if ms, err := strconv.Atoi(req.Options["timeout_ms"]); err == nil && ms > 0 {
		timeout = time.Duration(ms) * time.Millisecond
	}
	chunkSize := uint32(4096)
	if v, err := strconv.Atoi(req.Options["chunk_size"]); err == nil && v > 0 && v <= 0xffffff {
		chunkSize = uint32(v)
	}

	s := &session{
		events: make(chan apidoc.DebugEvent, 256),
		start:  time.Now(),
	}
	s.emit("out", "CONNECT", req.URL)
	dialer := &net.Dialer{Timeout: timeout}
	var conn *yrtmp.ClientConn
	if u.Scheme == "rtmps" {
		conn, err = yrtmp.TLSDial("rtmps", u.Host, nil, &tls.Config{ServerName: u.Hostname()})
	} else {
		conn, err = yrtmp.DialWithDialer(dialer, "rtmp", u.Host, nil)
	}
	if err != nil {
		return nil, fmt.Errorf("rtmp: dial %s: %w", u.Host, err)
	}
	if err := conn.Connect(nil); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("rtmp: connect: %w", err)
	}
	s.emit("in", "CONNECT accepted", "")

	stream, err := conn.CreateStream(nil, chunkSize)
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("rtmp: create stream: %w", err)
	}
	s.emit("in", fmt.Sprintf("CREATE_STREAM id=%d", stream.StreamID()), "")

	if err := stream.Publish(&message.NetStreamPublish{PublishingName: name, PublishingType: "live"}); err != nil {
		_ = stream.Close()
		_ = conn.Close()
		return nil, fmt.Errorf("rtmp: publish: %w", err)
	}
	s.emit("out", "PUBLISH "+name, "")
	s.conn = conn
	s.stream = stream
	return s, nil
}

type session struct {
	conn   *yrtmp.ClientConn
	stream *yrtmp.Stream
	events chan apidoc.DebugEvent
	start  time.Time
	once   sync.Once
	mu     sync.Mutex
}

func (s *session) Send(ctx context.Context, msg []byte, meta map[string]string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(msg) == 0 {
		return fmt.Errorf("rtmp: empty payload")
	}
	if s.stream == nil {
		return fmt.Errorf("rtmp: session closed")
	}
	kind := strings.ToLower(meta["type"])
	if kind == "" {
		kind = "video"
	}
	ts := uint32(time.Since(s.start) / time.Millisecond)

	s.mu.Lock()
	defer s.mu.Unlock()
	var err error
	switch kind {
	case "audio":
		err = s.stream.Write(audioChunkStream, ts, &message.AudioMessage{Payload: bytes.NewReader(msg)})
	case "video":
		err = s.stream.Write(videoChunkStream, ts, &message.VideoMessage{Payload: bytes.NewReader(msg)})
	default:
		return fmt.Errorf("rtmp: unsupported payload type %q", kind)
	}
	if err != nil {
		return err
	}
	s.emitPacket("out", fmt.Sprintf("%s len=%d", strings.ToUpper(kind), len(msg)), msg)
	return nil
}

func (s *session) Events() <-chan apidoc.DebugEvent { return s.events }

func (s *session) Close() error {
	s.once.Do(func() {
		if s.stream != nil {
			_ = s.stream.Close()
		}
		if s.conn != nil {
			_ = s.conn.Close()
		}
		s.emit("in", "CLOSE", "")
		close(s.events)
	})
	return nil
}

func (s *session) emit(dir, name, data string) {
	select {
	case s.events <- apidoc.DebugEvent{Time: time.Now(), Direction: dir, Name: name, Data: data, Encoding: "text"}:
	default:
	}
}

func (s *session) emitPacket(dir, name string, payload []byte) {
	select {
	case s.events <- apidoc.DebugEvent{
		Time: time.Now(), Direction: dir, Name: name,
		Data: hex.EncodeToString(payload), Encoding: "hex",
	}:
	default:
	}
}

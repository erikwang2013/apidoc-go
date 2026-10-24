package rtmp

import (
	"bytes"
	"context"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	yrtmp "github.com/yutopp/go-rtmp"
	"github.com/yutopp/go-rtmp/message"

	"github.com/erikwang2013/apidoc-go"
)

type handler struct {
	yrtmp.DefaultHandler
	published chan string
	video     chan []byte
	audio     chan []byte
}

func (h *handler) OnPublish(_ *yrtmp.StreamContext, _ uint32, cmd *message.NetStreamPublish) error {
	h.published <- cmd.PublishingName
	return nil
}

func (h *handler) OnVideo(_ uint32, payload io.Reader) error {
	b, err := io.ReadAll(payload)
	if err != nil {
		return err
	}
	h.video <- b
	return nil
}

func (h *handler) OnAudio(_ uint32, payload io.Reader) error {
	b, err := io.ReadAll(payload)
	if err != nil {
		return err
	}
	h.audio <- b
	return nil
}

func startServer(t *testing.T) (addr string, h *handler) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	h = &handler{
		published: make(chan string, 4),
		video:     make(chan []byte, 4),
		audio:     make(chan []byte, 4),
	}
	srv := yrtmp.NewServer(&yrtmp.ServerConfig{
		OnConnect: func(c net.Conn) (io.ReadWriteCloser, *yrtmp.ConnConfig) {
			return c, &yrtmp.ConnConfig{Handler: h}
		},
	})
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })
	return ln.Addr().String(), h
}

func waitString(t *testing.T, ch <-chan string) string {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-time.After(3 * time.Second):
		t.Fatal("timed out")
		return ""
	}
}

func waitBytes(t *testing.T, ch <-chan []byte) []byte {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-time.After(3 * time.Second):
		t.Fatal("timed out")
		return nil
	}
}

func TestPublishAndSend(t *testing.T) {
	addr, h := startServer(t)
	sess, err := New().Open(context.Background(), apidoc.DebugRequest{
		URL:     "rtmp://" + addr + "/live/test",
		Options: map[string]string{"timeout_ms": "3000"},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()

	if got := waitString(t, h.published); got != "live/test" {
		t.Fatalf("publish name = %q", got)
	}

	video := []byte{0x17, 0x00, 0x00, 0x00, 0x01}
	if err := sess.Send(context.Background(), video, map[string]string{"type": "video"}); err != nil {
		t.Fatal(err)
	}
	if got := waitBytes(t, h.video); !bytes.Equal(got, video) {
		t.Fatalf("video = %v, want %v", got, video)
	}

	audio := []byte{0xaf, 0x01}
	if err := sess.Send(context.Background(), audio, map[string]string{"type": "audio"}); err != nil {
		t.Fatal(err)
	}
	if got := waitBytes(t, h.audio); !bytes.Equal(got, audio) {
		t.Fatalf("audio = %v, want %v", got, audio)
	}

	if err := sess.Send(context.Background(), video, map[string]string{"type": "data"}); err == nil {
		t.Fatal("expected unsupported type error")
	}

	_ = sess.Close()
	var names []string
	for ev := range sess.Events() {
		names = append(names, ev.Direction+":"+ev.Name)
	}
	joined := strings.Join(names, ",")
	for _, want := range []string{"out:CONNECT", "in:CONNECT accepted", "PUBLISH live/test", "out:VIDEO", "out:AUDIO", "in:CLOSE"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("events %q missing %q", joined, want)
		}
	}
}

func TestExecPublishes(t *testing.T) {
	addr, h := startServer(t)
	res, err := New().Exec(context.Background(), apidoc.DebugRequest{
		Method:  "PUBLISH",
		URL:     "rtmp://" + addr + "/live/exec",
		Body:    []byte{0x17, 0x01},
		Options: map[string]string{"timeout_ms": "3000"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != "PUBLISHED" {
		t.Fatalf("status = %q", res.Status)
	}
	if got := waitString(t, h.published); got != "live/exec" {
		t.Fatalf("publish name = %q", got)
	}
}

func TestBadURL(t *testing.T) {
	if _, err := New().Open(context.Background(), apidoc.DebugRequest{URL: "http://example.com/live"}); err == nil {
		t.Fatal("expected scheme error")
	}
}

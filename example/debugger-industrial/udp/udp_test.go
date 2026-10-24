package udp

import (
	"context"
	"net"
	"testing"

	"github.com/erikwang2013/apidoc-go"
)

func TestEcho(t *testing.T) {
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer pc.Close()
	go func() {
		buf := make([]byte, 256)
		n, addr, err := pc.ReadFrom(buf)
		if err != nil {
			return
		}
		_, _ = pc.WriteTo(buf[:n], addr)
	}()

	resp, err := New().Exec(context.Background(), apidoc.DebugRequest{
		URL:      "udp://" + pc.LocalAddr().String(),
		Body:     []byte("ping"),
		Encoding: "text",
		Options:  map[string]string{"timeout_ms": "500"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if string(resp.Body) != "ping" || resp.Status != "OK" {
		t.Fatalf("response = %q %q", resp.Body, resp.Status)
	}
}

func TestNoReplyTimesOut(t *testing.T) {
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer pc.Close()

	resp, err := New().Exec(context.Background(), apidoc.DebugRequest{
		URL:     "udp://" + pc.LocalAddr().String(),
		Body:    []byte("x"),
		Options: map[string]string{"timeout_ms": "50"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Status != "TIMEOUT" {
		t.Fatalf("status = %q", resp.Status)
	}
}

func TestBadURL(t *testing.T) {
	if _, err := New().Exec(context.Background(), apidoc.DebugRequest{URL: "tcp://x:1"}); err == nil {
		t.Fatal("want error for non-udp url")
	}
}

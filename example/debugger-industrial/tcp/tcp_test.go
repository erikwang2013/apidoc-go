package tcp

import (
	"context"
	"io"
	"net"
	"testing"

	"github.com/erikwang2013/apidoc-go"
)

func TestEcho(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		_, _ = io.Copy(c, c)
	}()

	resp, err := New().Exec(context.Background(), apidoc.DebugRequest{
		URL:      "tcp://" + ln.Addr().String(),
		Body:     []byte{0x00, 0xff, 0x10},
		Encoding: "hex",
		Options:  map[string]string{"timeout_ms": "100"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if string(resp.Body) != string([]byte{0x00, 0xff, 0x10}) || resp.Status != "OK" {
		t.Fatalf("response = %x %q", resp.Body, resp.Status)
	}
}

func TestBadURL(t *testing.T) {
	if _, err := New().Exec(context.Background(), apidoc.DebugRequest{URL: "udp://x:1"}); err == nil {
		t.Fatal("want error for non-tcp url")
	}
}

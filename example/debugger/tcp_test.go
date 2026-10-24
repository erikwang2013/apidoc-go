package debugger

import (
	"context"
	"io"
	"net"
	"testing"

	"github.com/erikwang2013/apidoc-go"
)

func TestTCPDebuggerEcho(t *testing.T) {
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

	resp, err := TCP().Exec(context.Background(), apidoc.DebugRequest{
		URL:      "tcp://" + ln.Addr().String(),
		Body:     []byte("hello"),
		Encoding: "text",
		Options:  map[string]string{"timeout_ms": "100"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if string(resp.Body) != "hello" || resp.Status != "OK" {
		t.Fatalf("response = %+v", resp)
	}
}

func TestTCPDebuggerBadURL(t *testing.T) {
	if _, err := TCP().Exec(context.Background(), apidoc.DebugRequest{URL: "http://x"}); err == nil {
		t.Fatal("want error for non-tcp url")
	}
}

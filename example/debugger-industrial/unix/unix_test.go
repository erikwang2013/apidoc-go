package unix

import (
	"context"
	"io"
	"net"
	"path/filepath"
	"testing"

	"github.com/erikwang2013/apidoc-go"
)

func TestEcho(t *testing.T) {
	path := filepath.Join(t.TempDir(), "echo.sock")
	ln, err := net.Listen("unix", path)
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
		URL:      "unix://" + path,
		Body:     []byte("hello"),
		Encoding: "text",
		Options:  map[string]string{"timeout_ms": "100"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if string(resp.Body) != "hello" || resp.Status != "OK" {
		t.Fatalf("response = %q %q", resp.Body, resp.Status)
	}
}

func TestBadURL(t *testing.T) {
	if _, err := New().Exec(context.Background(), apidoc.DebugRequest{URL: "tcp://x:1"}); err == nil {
		t.Fatal("want error for non-unix url")
	}
}

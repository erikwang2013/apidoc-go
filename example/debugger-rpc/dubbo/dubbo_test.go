package dubbodbg

import (
	"context"
	"errors"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	hessian "github.com/apache/dubbo-go-hessian2"
	"github.com/erikwang2013/apidoc-go"
)

type captured struct {
	flag         byte
	dubboVersion string
	service      string
	version      string
	method       string
	types        string
	args         []any
}

// startServer accepts one Dubbo request and answers with flag+value, so the
// adapter is tested against real frames and the real Hessian2 codec.
func startServer(t *testing.T, bodyFlag byte, value any) (string, <-chan captured) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	got := make(chan captured, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
		flag, _, id, body, err := readFrame(conn)
		if err != nil {
			got <- captured{}
			return
		}
		d := hessian.NewDecoder(body)
		str := func() string {
			v, err := d.Decode()
			if err != nil {
				return ""
			}
			s, _ := v.(string)
			return s
		}
		c := captured{flag: flag, dubboVersion: str(), service: str(), version: str(), method: str(), types: str()}
		for {
			v, err := d.Decode()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				break
			}
			c.args = append(c.args, v)
		}
		got <- c

		e := hessian.NewEncoder()
		_ = e.Encode(value)
		out := append([]byte{bodyFlag}, e.Buffer()...)
		_ = writeFrame(conn, responseFlag, statusOK, id, out)
	}()
	t.Cleanup(func() { ln.Close() })
	return ln.Addr().String(), got
}

func TestCall(t *testing.T) {
	addr, got := startServer(t, responseValue, "hello")
	resp, err := New().Exec(context.Background(), apidoc.DebugRequest{
		URL:  "dubbo://" + addr + "/com.example.Demo/sayHello?version=1.0.0",
		Body: []byte(`["world"]`),
	})
	if err != nil {
		t.Fatal(err)
	}
	c := <-got
	if c.flag != requestFlag {
		t.Fatalf("request flag = %#x", c.flag)
	}
	if c.dubboVersion != "2.0.2" || c.service != "com.example.Demo" || c.method != "sayHello" || c.version != "1.0.0" {
		t.Fatalf("request = %+v", c)
	}
	if c.types != "Ljava/lang/String;" || len(c.args) != 1 || c.args[0] != "world" {
		t.Fatalf("args = %v types = %q", c.args, c.types)
	}
	if resp.Status != "OK" || strings.TrimSpace(string(resp.Body)) != `"hello"` {
		t.Fatalf("status=%q body=%q", resp.Status, resp.Body)
	}
}

func TestRemoteException(t *testing.T) {
	addr, _ := startServer(t, responseWithException, "service unavailable")
	resp, err := New().Exec(context.Background(), apidoc.DebugRequest{
		URL:  "dubbo://" + addr + "/com.example.Demo/sayHello",
		Body: []byte(`[]`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Status != "ERROR" || !strings.Contains(string(resp.Body), "service unavailable") {
		t.Fatalf("status=%q body=%q", resp.Status, resp.Body)
	}
}

func TestDecodeResponseFlags(t *testing.T) {
	if v, remote, err := decodeResponse([]byte{responseNull}); err != nil || v != nil || remote {
		t.Fatalf("null: %v %v %v", v, remote, err)
	}
	e := hessian.NewEncoder()
	_ = e.Encode(int64(7))
	if v, remote, err := decodeResponse(append([]byte{responseValue}, e.Buffer()...)); err != nil || v.(int64) != 7 || remote {
		t.Fatalf("value: %v %v %v", v, remote, err)
	}
	if _, _, err := decodeResponse([]byte{0x7f}); err == nil {
		t.Fatal("want error for unknown flag")
	}
}

func TestBadURL(t *testing.T) {
	if _, err := New().Exec(context.Background(), apidoc.DebugRequest{URL: "grpc://localhost/x/y"}); err == nil {
		t.Fatal("want error for non-dubbo url")
	}
}

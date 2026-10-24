package coap

import (
	"bytes"
	"context"
	"io"
	"testing"
	"time"

	"github.com/erikwang2013/apidoc-go"
	"github.com/plgd-dev/go-coap/v3/message"
	"github.com/plgd-dev/go-coap/v3/message/codes"
	"github.com/plgd-dev/go-coap/v3/mux"
	coapnet "github.com/plgd-dev/go-coap/v3/net"
	"github.com/plgd-dev/go-coap/v3/options"
	"github.com/plgd-dev/go-coap/v3/udp"
)

func serveCoAP(t *testing.T) string {
	t.Helper()
	router := mux.NewRouter()
	router.HandleFunc("/hello", func(w mux.ResponseWriter, _ *mux.Message) {
		_ = w.SetResponse(codes.Content, message.TextPlain, bytes.NewReader([]byte("hello")))
	})
	router.HandleFunc("/echo", func(w mux.ResponseWriter, r *mux.Message) {
		body, _ := io.ReadAll(r.Body())
		_ = w.SetResponse(codes.Content, message.AppJSON, bytes.NewReader(body))
	})
	router.HandleFunc("/slow", func(w mux.ResponseWriter, _ *mux.Message) {
		time.Sleep(300 * time.Millisecond)
		_ = w.SetResponse(codes.Content, message.TextPlain, bytes.NewReader([]byte("late")))
	})
	l, err := coapnet.NewListenUDP("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s := udp.NewServer(options.WithMux(router))
	go func() { _ = s.Serve(l) }()
	t.Cleanup(func() { s.Stop(); l.Close() })
	return l.LocalAddr().String()
}

func TestGet(t *testing.T) {
	addr := serveCoAP(t)
	resp, err := New().Exec(context.Background(), apidoc.DebugRequest{
		Method: "GET", URL: "coap://" + addr + "/hello",
		Options: map[string]string{"timeout_ms": "2000"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if string(resp.Body) != "hello" || resp.Status != "Content" || resp.Encoding != "text" {
		t.Fatalf("response = %+v (%q)", resp, resp.Body)
	}
}

func TestPostJSON(t *testing.T) {
	addr := serveCoAP(t)
	resp, err := New().Exec(context.Background(), apidoc.DebugRequest{
		Method: "POST", URL: "coap://" + addr + "/echo",
		Body:    []byte(`{"x":1}`),
		Options: map[string]string{"content_format": "50", "timeout_ms": "2000"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if string(resp.Body) != `{"x":1}` || resp.Encoding != "json" {
		t.Fatalf("response = %+v (%q)", resp, resp.Body)
	}
}

func TestTimeout(t *testing.T) {
	addr := serveCoAP(t)
	_, err := New().Exec(context.Background(), apidoc.DebugRequest{
		Method: "GET", URL: "coap://" + addr + "/slow",
		Options: map[string]string{"timeout_ms": "50"},
	})
	if err == nil {
		t.Fatal("want timeout error")
	}
}

func TestBadURL(t *testing.T) {
	if _, err := New().Exec(context.Background(), apidoc.DebugRequest{
		Method: "GET", URL: "http://127.0.0.1:1/x",
	}); err == nil {
		t.Fatal("want error for non-coap url")
	}
}

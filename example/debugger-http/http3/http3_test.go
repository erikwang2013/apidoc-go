package http3

import (
	"context"
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/erikwang2013/apidoc-go"
	quichttp3 "github.com/quic-go/quic-go/http3"
)

func testCert(t *testing.T) tls.Certificate {
	t.Helper()
	ts := httptest.NewTLSServer(http.NotFoundHandler())
	cert := ts.TLS.Certificates[0]
	ts.Close()
	return cert
}

func TestHTTP3Exec(t *testing.T) {
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	var gotProto, gotQuery string
	srv := &quichttp3.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotProto = r.Proto
			gotQuery = r.URL.Query().Get("q")
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"proto":"`+r.Proto+`"}`)
		}),
		TLSConfig: &tls.Config{Certificates: []tls.Certificate{testCert(t)}},
	}
	go func() { _ = srv.Serve(conn) }()
	defer srv.Close()

	// h3s:// must be rewritten to https:// by the adapter.
	resp, err := New().Exec(context.Background(), apidoc.DebugRequest{
		URL:      "h3s://" + conn.LocalAddr().String() + "/echo",
		Method:   "GET",
		Encoding: "json",
		Params:   []apidoc.DebugValue{{Name: "q", In: "query", Value: "x y"}},
		Options:  map[string]string{"insecure_skip_verify": "true", "timeout_ms": "10000"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if gotProto != "HTTP/3.0" {
		t.Errorf("server saw %s, want HTTP/3.0", gotProto)
	}
	if gotQuery != "x y" {
		t.Errorf("server saw query %q", gotQuery)
	}
	if resp.Status != "200 OK" || !strings.Contains(string(resp.Body), "HTTP/3.0") {
		t.Fatalf("response = %+v", resp)
	}
}

func TestHTTP3RejectsPlainHTTP(t *testing.T) {
	if _, err := New().Exec(context.Background(), apidoc.DebugRequest{
		URL: "http://example.invalid",
	}); err == nil {
		t.Error("want error for non-TLS URL")
	}
}

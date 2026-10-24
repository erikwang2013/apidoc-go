package lwm2m

import (
	"bytes"
	"context"
	"io"
	"sync"
	"testing"

	"github.com/erikwang2013/apidoc-go"
	"github.com/plgd-dev/go-coap/v3/message"
	"github.com/plgd-dev/go-coap/v3/message/codes"
	"github.com/plgd-dev/go-coap/v3/mux"
	coapnet "github.com/plgd-dev/go-coap/v3/net"
	"github.com/plgd-dev/go-coap/v3/options"
	"github.com/plgd-dev/go-coap/v3/udp"
)

type state struct {
	mu    sync.Mutex
	value string
	cf    message.MediaType
}

func serveLwM2M(t *testing.T) (string, *state) {
	t.Helper()
	st := &state{}
	router := mux.NewRouter()
	router.HandleFunc("/3303/0/5700", func(w mux.ResponseWriter, r *mux.Message) {
		switch r.Code() {
		case codes.GET:
			_ = w.SetResponse(codes.Content, message.TextPlain, bytes.NewReader([]byte("21.5")))
		case codes.PUT:
			body, _ := io.ReadAll(r.Body())
			cf, _ := r.Options().ContentFormat()
			st.mu.Lock()
			st.value, st.cf = string(body), cf
			st.mu.Unlock()
			_ = w.SetResponse(codes.Changed, message.TextPlain, nil)
		}
	})
	router.HandleFunc("/3303/0/5605", func(w mux.ResponseWriter, _ *mux.Message) {
		_ = w.SetResponse(codes.Changed, message.TextPlain, nil)
	})
	router.HandleFunc("/3303/0", func(w mux.ResponseWriter, _ *mux.Message) {
		_ = w.SetResponse(codes.Content, message.AppLinkFormat, bytes.NewReader([]byte("</3303/0/5700>")))
	})
	l, err := coapnet.NewListenUDP("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s := udp.NewServer(options.WithMux(router))
	go func() { _ = s.Serve(l) }()
	t.Cleanup(func() { s.Stop(); l.Close() })
	return l.LocalAddr().String(), st
}

func testRequest(addr, method, path string) apidoc.DebugRequest {
	return apidoc.DebugRequest{
		Method: method, URL: "lwm2m://" + addr + path,
		Options: map[string]string{"timeout_ms": "2000"},
	}
}

func TestRead(t *testing.T) {
	addr, _ := serveLwM2M(t)
	resp, err := New().Exec(context.Background(), testRequest(addr, "READ", "/3303/0/5700"))
	if err != nil {
		t.Fatal(err)
	}
	if string(resp.Body) != "21.5" || resp.Status != "Content" {
		t.Fatalf("response = %+v (%q)", resp, resp.Body)
	}
}

func TestWrite(t *testing.T) {
	addr, st := serveLwM2M(t)
	req := testRequest(addr, "WRITE", "/3303/0/5700")
	req.Body = []byte("22.0")
	req.Options["content_format"] = "0"
	resp, err := New().Exec(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Status != "Changed" {
		t.Fatalf("status = %q", resp.Status)
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.value != "22.0" || st.cf != message.TextPlain {
		t.Fatalf("server saw value=%q cf=%v", st.value, st.cf)
	}
}

func TestExecute(t *testing.T) {
	addr, _ := serveLwM2M(t)
	resp, err := New().Exec(context.Background(), testRequest(addr, "EXECUTE", "/3303/0/5605"))
	if err != nil {
		t.Fatal(err)
	}
	if resp.Status != "Changed" {
		t.Fatalf("status = %q", resp.Status)
	}
}

func TestDiscover(t *testing.T) {
	addr, _ := serveLwM2M(t)
	resp, err := New().Exec(context.Background(), testRequest(addr, "DISCOVER", "/3303/0"))
	if err != nil {
		t.Fatal(err)
	}
	if string(resp.Body) != "</3303/0/5700>" || resp.Encoding != "text" {
		t.Fatalf("response = %+v (%q)", resp, resp.Body)
	}
}

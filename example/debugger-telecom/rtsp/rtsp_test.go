package rtsp

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/bluenviron/gortsplib/v4"
	"github.com/bluenviron/gortsplib/v4/pkg/base"
	"github.com/bluenviron/gortsplib/v4/pkg/description"
	"github.com/bluenviron/gortsplib/v4/pkg/format"
	"github.com/pion/rtp"

	"github.com/erikwang2013/apidoc-go"
)

type handler struct {
	stream *gortsplib.ServerStream
}

func (h *handler) OnDescribe(*gortsplib.ServerHandlerOnDescribeCtx) (*base.Response, *gortsplib.ServerStream, error) {
	return &base.Response{StatusCode: base.StatusOK}, h.stream, nil
}

func (h *handler) OnSetup(*gortsplib.ServerHandlerOnSetupCtx) (*base.Response, *gortsplib.ServerStream, error) {
	return &base.Response{StatusCode: base.StatusOK}, h.stream, nil
}

func (h *handler) OnPlay(*gortsplib.ServerHandlerOnPlayCtx) (*base.Response, error) {
	return &base.Response{StatusCode: base.StatusOK}, nil
}

type testServer struct {
	srv    *gortsplib.Server
	stream *gortsplib.ServerStream
	desc   *description.Session
	addr   string
}

func newServer(t *testing.T) *testServer {
	t.Helper()
	ts := &testServer{}
	h := &handler{}
	srv := &gortsplib.Server{RTSPAddress: "127.0.0.1:0", Handler: h}
	srv.Listen = func(network, address string) (net.Listener, error) {
		ln, err := net.Listen(network, address)
		if err == nil {
			ts.addr = ln.Addr().String()
		}
		return ln, err
	}
	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(srv.Close)
	if ts.addr == "" {
		t.Fatal("server did not bind")
	}
	ts.srv = srv
	ts.desc = &description.Session{Title: "test", Medias: []*description.Media{{
		Type:    description.MediaTypeAudio,
		Formats: []format.Format{&format.G711{PayloadTyp: 8, MULaw: true, SampleRate: 8000, ChannelCount: 1}},
	}}}
	ts.stream = &gortsplib.ServerStream{Server: srv, Desc: ts.desc}
	if err := ts.stream.Initialize(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(ts.stream.Close)
	h.stream = ts.stream
	return ts
}

func TestExecDescribe(t *testing.T) {
	ts := newServer(t)
	res, err := New().Exec(context.Background(), apidoc.DebugRequest{
		Method: "DESCRIBE",
		URL:    "rtsp://" + ts.addr + "/test",
		Options: map[string]string{
			"transport": "tcp", "timeout_ms": "2000",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != "200 OK" {
		t.Fatalf("status = %q", res.Status)
	}
	if !strings.Contains(string(res.Body), "m=audio") {
		t.Fatalf("sdp body = %q", res.Body)
	}
	var sawOut, sawIn bool
	for _, ev := range res.Events {
		if ev.Direction == "out" && strings.Contains(ev.Data, "DESCRIBE") {
			sawOut = true
		}
		if ev.Direction == "in" && strings.Contains(ev.Data, "200 OK") {
			sawIn = true
		}
	}
	if !sawOut || !sawIn {
		t.Fatalf("transcript missing: %+v", res.Events)
	}
}

func TestStreamPlaySession(t *testing.T) {
	ts := newServer(t)
	sess, err := New().Open(context.Background(), apidoc.DebugRequest{
		URL:     "rtsp://" + ts.addr + "/test",
		Options: map[string]string{"transport": "tcp", "timeout_ms": "2000"},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()

	pkt := &rtp.Packet{
		Header:  rtp.Header{Version: 2, PayloadType: 8, SequenceNumber: 11, Timestamp: 160, SSRC: 0xabcdef},
		Payload: []byte{0x11, 0x22},
	}
	if err := ts.stream.WritePacketRTP(ts.desc.Medias[0], pkt); err != nil {
		t.Fatal(err)
	}

	timeout := time.After(3 * time.Second)
	for {
		select {
		case ev, ok := <-sess.Events():
			if !ok {
				t.Fatal("events closed before RTP packet")
			}
			if ev.Direction == "in" && strings.Contains(ev.Name, "RTP") && strings.Contains(ev.Name, "seq=11") {
				return
			}
		case <-timeout:
			t.Fatal("timed out waiting for RTP event")
		}
	}
}

func TestExecPlayWaitsForMedia(t *testing.T) {
	ts := newServer(t)
	go func() {
		time.Sleep(200 * time.Millisecond)
		_ = ts.stream.WritePacketRTP(ts.desc.Medias[0], &rtp.Packet{
			Header: rtp.Header{Version: 2, PayloadType: 8, SequenceNumber: 2, SSRC: 1},
		})
	}()
	res, err := New().Exec(context.Background(), apidoc.DebugRequest{
		Method: "PLAY", URL: "rtsp://" + ts.addr + "/test",
		Options: map[string]string{"transport": "tcp", "timeout_ms": "2000", "reply_ms": "1500"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != "200 OK" {
		t.Fatalf("status = %q", res.Status)
	}
	var media bool
	for _, ev := range res.Events {
		if strings.Contains(ev.Name, "RTP") && strings.Contains(ev.Name, "seq=2") {
			media = true
		}
	}
	if !media {
		t.Fatalf("no media event: %+v", res.Events)
	}
}

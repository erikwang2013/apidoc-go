package webrtc

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	pion "github.com/pion/webrtc/v4"

	"github.com/erikwang2013/apidoc-go"
)

// signalServer is a local SDP signaling endpoint backed by a real pion peer
// that echoes every data-channel message.
type signalServer struct {
	*httptest.Server
	mu  sync.Mutex
	pcs []*pion.PeerConnection
}

func newSignalServer(t *testing.T) *signalServer {
	t.Helper()
	ss := &signalServer{}
	ss.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.Header.Get("Content-Type") != "application/sdp" {
			http.Error(w, "want POST application/sdp", http.StatusBadRequest)
			return
		}
		offerSDP, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		pc, err := pion.NewPeerConnection(pion.Configuration{})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		ss.mu.Lock()
		ss.pcs = append(ss.pcs, pc)
		ss.mu.Unlock()

		pc.OnDataChannel(func(dc *pion.DataChannel) {
			dc.OnMessage(func(m pion.DataChannelMessage) {
				_ = dc.SendText("echo:" + string(m.Data))
			})
		})
		if err := pc.SetRemoteDescription(pion.SessionDescription{Type: pion.SDPTypeOffer, SDP: string(offerSDP)}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		answer, err := pc.CreateAnswer(nil)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if err := pc.SetLocalDescription(answer); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		select {
		case <-pion.GatheringCompletePromise(pc):
		case <-time.After(10 * time.Second):
			http.Error(w, "ice gathering timed out", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/sdp")
		_, _ = w.Write([]byte(pc.LocalDescription().SDP))
	}))
	t.Cleanup(func() {
		ss.Server.Close()
		ss.mu.Lock()
		defer ss.mu.Unlock()
		for _, pc := range ss.pcs {
			_ = pc.Close()
		}
	})
	return ss
}

func rtcURL(ss *signalServer) string {
	return "webrtc://" + strings.TrimPrefix(ss.URL, "http://") + "/signal"
}

func nextEvent(t *testing.T, s apidoc.Session) apidoc.DebugEvent {
	t.Helper()
	select {
	case ev, ok := <-s.Events():
		if !ok {
			t.Fatal("events channel closed")
		}
		return ev
	case <-time.After(15 * time.Second):
		t.Fatal("timed out waiting for event")
		return apidoc.DebugEvent{}
	}
}

func TestSessionEcho(t *testing.T) {
	ss := newSignalServer(t)
	s, err := New().Open(context.Background(), apidoc.DebugRequest{
		URL:     rtcURL(ss),
		Options: map[string]string{"timeout_ms": "15000"},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	if err := s.Send(context.Background(), []byte("hello"), nil); err != nil {
		t.Fatal(err)
	}
	ev := nextEvent(t, s)
	if ev.Direction != "in" || ev.Data != "echo:hello" || ev.Encoding != "text" {
		t.Fatalf("event = %+v", ev)
	}
}

func TestExecOneShot(t *testing.T) {
	ss := newSignalServer(t)
	resp, err := New().Exec(context.Background(), apidoc.DebugRequest{
		URL:     rtcURL(ss),
		Body:    []byte("hi"),
		Options: map[string]string{"timeout_ms": "15000", "read_timeout_ms": "15000"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Status != "OK" || string(resp.Body) != "echo:hi" {
		t.Fatalf("response = %+v", resp)
	}
}

func TestBadURL(t *testing.T) {
	if _, err := New().Open(context.Background(), apidoc.DebugRequest{URL: "ftp://x"}); err == nil {
		t.Fatal("want error for unsupported scheme")
	}
}

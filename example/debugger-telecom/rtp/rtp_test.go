package rtp

import (
	"context"
	"encoding/hex"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/pion/rtp"

	"github.com/erikwang2013/apidoc-go"
)

func echoServer(t *testing.T) string {
	t.Helper()
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = pc.Close() })
	go func() {
		buf := make([]byte, 2048)
		for {
			n, addr, err := pc.ReadFrom(buf)
			if err != nil {
				return
			}
			_, _ = pc.WriteTo(buf[:n], addr)
		}
	}()
	return pc.LocalAddr().String()
}

func testPacket(t *testing.T) []byte {
	t.Helper()
	p := &rtp.Packet{
		Header:  rtp.Header{Version: 2, PayloadType: 96, SequenceNumber: 7, Timestamp: 1000, SSRC: 0x1234},
		Payload: []byte{0xaa, 0xbb, 0xcc},
	}
	raw, err := p.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestSessionSendReceive(t *testing.T) {
	addr := echoServer(t)
	sess, err := New().Open(context.Background(), apidoc.DebugRequest{URL: "rtp://" + addr})
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()

	raw := testPacket(t)
	if err := sess.Send(context.Background(), raw, nil); err != nil {
		t.Fatal(err)
	}

	var out, in *apidoc.DebugEvent
	timeout := time.After(2 * time.Second)
	for out == nil || in == nil {
		select {
		case ev := <-sess.Events():
			if ev.Direction == "out" {
				out = &ev
			} else {
				in = &ev
			}
		case <-timeout:
			t.Fatalf("timed out: out=%v in=%v", out, in)
		}
	}
	if !strings.Contains(in.Name, "seq=7") {
		t.Fatalf("in event = %q", in.Name)
	}
	got, err := hex.DecodeString(in.Data)
	if err != nil || string(got) != string(raw) {
		t.Fatalf("in data mismatch: %v %v", got, err)
	}
}

func TestExecEcho(t *testing.T) {
	addr := echoServer(t)
	res, err := New().Exec(context.Background(), apidoc.DebugRequest{
		Method: "SEND", URL: "rtp://" + addr, Body: testPacket(t),
		Options: map[string]string{"timeout_ms": "1000"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != "OK" {
		t.Fatalf("status = %q", res.Status)
	}
	if len(res.Events) != 2 {
		t.Fatalf("events = %d, want 2", len(res.Events))
	}
}

func TestInvalidPacket(t *testing.T) {
	sess, err := New().Open(context.Background(), apidoc.DebugRequest{URL: "rtp://" + echoServer(t)})
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()
	if err := sess.Send(context.Background(), []byte{0x80}, nil); err == nil {
		t.Fatal("expected invalid packet error")
	}
	if err := sess.Send(context.Background(), []byte{0x80}, map[string]string{"raw": "1"}); err != nil {
		t.Fatalf("raw send: %v", err)
	}
}

func TestBadURL(t *testing.T) {
	if _, err := New().Open(context.Background(), apidoc.DebugRequest{URL: "http://example.com/x"}); err == nil {
		t.Fatal("expected scheme error")
	}
}

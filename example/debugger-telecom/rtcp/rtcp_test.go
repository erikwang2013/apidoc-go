package rtcp

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"

	pionrtcp "github.com/pion/rtcp"

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

func testReport(t *testing.T) []byte {
	t.Helper()
	rr := &pionrtcp.ReceiverReport{
		SSRC: 0x11223344,
		Reports: []pionrtcp.ReceptionReport{{
			SSRC: 0x55667788, LastSequenceNumber: 4242, Jitter: 12,
		}},
	}
	raw, err := rr.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestSessionSendReceive(t *testing.T) {
	addr := echoServer(t)
	sess, err := New().Open(context.Background(), apidoc.DebugRequest{URL: "rtcp://" + addr})
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()

	raw := testReport(t)
	if err := sess.Send(context.Background(), raw, nil); err != nil {
		t.Fatal(err)
	}
	var in *apidoc.DebugEvent
	timeout := time.After(2 * time.Second)
	for in == nil {
		select {
		case ev := <-sess.Events():
			if ev.Direction == "in" {
				in = &ev
			}
		case <-timeout:
			t.Fatal("timed out waiting for echo")
		}
	}
	if !strings.Contains(in.Name, "ReceiverReport") {
		t.Fatalf("in event = %q", in.Name)
	}
	if in.Encoding != "hex" || in.Data == "" {
		t.Fatalf("in event = %+v", in)
	}
}

func TestExecEcho(t *testing.T) {
	addr := echoServer(t)
	res, err := New().Exec(context.Background(), apidoc.DebugRequest{
		Method: "SEND", URL: "rtcp://" + addr, Body: testReport(t),
		Options: map[string]string{"timeout_ms": "1000"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != "OK" {
		t.Fatalf("status = %q", res.Status)
	}
	var names []string
	for _, ev := range res.Events {
		names = append(names, ev.Direction+":"+ev.Name)
	}
	if len(names) != 2 || !strings.HasPrefix(names[1], "in:") {
		t.Fatalf("events = %v", names)
	}
}

func TestInvalidPacket(t *testing.T) {
	sess, err := New().Open(context.Background(), apidoc.DebugRequest{URL: "rtcp://" + echoServer(t)})
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()
	if err := sess.Send(context.Background(), []byte{0xff}, nil); err == nil {
		t.Fatal("expected invalid packet error")
	}
	if err := sess.Send(context.Background(), []byte{0xff}, map[string]string{"raw": "true"}); err != nil {
		t.Fatalf("raw send: %v", err)
	}
}

package sip

import (
	"context"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/emiago/sipgo/sip"

	"github.com/erikwang2013/apidoc-go"
)

const testSDP = "v=0\r\no=- 0 0 IN IP4 127.0.0.1\r\ns=test\r\nt=0 0\r\nm=audio 9 RTP/AVP 0\r\n"

func startServer(t *testing.T) (addr string, seen <-chan string) {
	t.Helper()
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	udp := pc.(*net.UDPConn)
	ch := make(chan string, 16)
	host, portStr, _ := net.SplitHostPort(udp.LocalAddr().String())
	port, _ := strconv.Atoi(portStr)

	go func() {
		buf := make([]byte, 65535)
		for {
			n, remote, err := udp.ReadFromUDP(buf)
			if err != nil {
				return
			}
			msg, err := sip.ParseMessage(buf[:n])
			if err != nil {
				continue
			}
			req, ok := msg.(*sip.Request)
			if !ok {
				continue
			}
			switch req.Method {
			case sip.OPTIONS:
				res := sip.NewResponseFromRequest(req, 200, "OK", nil)
				_, _ = udp.WriteToUDP([]byte(res.String()), remote)
				ch <- "OPTIONS"
			case sip.INVITE:
				res := sip.NewResponseFromRequest(req, 200, "OK", []byte(testSDP))
				res.AppendHeader(sip.NewHeader("Content-Type", "application/sdp"))
				res.AppendHeader(&sip.ContactHeader{Address: sip.Uri{Scheme: "sip", Host: host, Port: port}})
				_, _ = udp.WriteToUDP([]byte(res.String()), remote)
				ch <- "INVITE"
			case sip.ACK:
				ch <- "ACK"
			case sip.BYE:
				res := sip.NewResponseFromRequest(req, 200, "OK", nil)
				_, _ = udp.WriteToUDP([]byte(res.String()), remote)
				ch <- "BYE"
			}
		}
	}()
	t.Cleanup(func() { _ = pc.Close() })
	return udp.LocalAddr().String(), ch
}

func TestExecOptions(t *testing.T) {
	addr, seen := startServer(t)
	res, err := New().Exec(context.Background(), apidoc.DebugRequest{
		Method: "OPTIONS",
		URL:    "sip://alice@" + addr,
		Options: map[string]string{
			"timeout_ms": "3000",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != "200 OK" {
		t.Fatalf("status = %q", res.Status)
	}
	if len(res.Events) != 2 {
		t.Fatalf("events = %d: %+v", len(res.Events), res.Events)
	}
	if !strings.Contains(res.Events[0].Data, "OPTIONS sip:alice@") {
		t.Fatalf("request = %q", res.Events[0].Data)
	}
	select {
	case got := <-seen:
		if got != "OPTIONS" {
			t.Fatalf("server saw %q", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("server did not see OPTIONS")
	}
}

func TestExecInvite(t *testing.T) {
	addr, seen := startServer(t)
	res, err := New().Exec(context.Background(), apidoc.DebugRequest{
		Method: "INVITE",
		URL:    "sip://bob@" + addr,
		Options: map[string]string{
			"username":   "alice",
			"timeout_ms": "5000",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != "200 OK" {
		t.Fatalf("status = %q", res.Status)
	}
	var names []string
	for _, ev := range res.Events {
		names = append(names, ev.Direction+":"+ev.Name)
	}
	want := []string{"out:INVITE", "in:SIP 200", "out:ACK", "out:BYE"}
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Fatalf("events = %v, want %v", names, want)
	}
	for _, want := range []string{"INVITE", "ACK", "BYE"} {
		select {
		case got := <-seen:
			if got != want {
				t.Fatalf("server saw %q, want %q", got, want)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("server did not see %s", want)
		}
	}
}

func TestBadURL(t *testing.T) {
	if _, err := New().Exec(context.Background(), apidoc.DebugRequest{
		Method: "OPTIONS", URL: "http://example.com",
	}); err == nil {
		t.Fatal("expected scheme error")
	}
}

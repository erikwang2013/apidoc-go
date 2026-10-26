//go:build apidoc_debug_sockets || apidoc_debug_all

package uiapi

import (
	"encoding/json"
	"net"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRelayTCPUnary(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		buf := make([]byte, 4)
		if _, err := conn.Read(buf); err == nil {
			_, _ = conn.Write([]byte("pong"))
		}
	}()

	target := "tcp://" + ln.Addr().String()
	h, id := debugStoreHandler(t, "tcp", "POST", target, DebugConfig{
		EnableServerSide: true, AllowPrivateNetworks: true, TimeoutMS: 2000,
	})
	data := debugSendData(t, h, id, `{"payload":"ping","encoding":"utf8","fields":{"expect_bytes":"4","read_ms":"500"}}`)
	if data["ok"] != true || data["status"] != float64(0) {
		t.Fatalf("unexpected result: %+v", data)
	}
	headers := data["headers"].(map[string]any)
	if headers["x-peer"] != ln.Addr().String() || headers["x-datagrams"] != "1" {
		t.Fatalf("unexpected headers: %+v", headers)
	}
	if data["body"] != "pong" || data["encoding"] != "text" {
		t.Fatalf("unexpected body: %+v", data)
	}
}

func TestRelayUDPUnary(t *testing.T) {
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer pc.Close()
	go func() {
		buf := make([]byte, 1024)
		n, addr, err := pc.ReadFrom(buf)
		if err != nil {
			return
		}
		_, _ = pc.WriteTo(append([]byte("echo:"), buf[:n]...), addr)
	}()

	target := "udp://" + pc.LocalAddr().String()
	h, id := debugStoreHandler(t, "udp", "POST", target, DebugConfig{
		EnableServerSide: true, AllowPrivateNetworks: true, TimeoutMS: 2000,
	})
	data := debugSendData(t, h, id, `{"payload":"hi","encoding":"utf8","fields":{"read_ms":"500"}}`)
	if data["ok"] != true || data["body"] != "echo:hi" {
		t.Fatalf("udp echo: %+v", data)
	}
}

func TestRelayUDPMultipleDatagrams(t *testing.T) {
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer pc.Close()
	go func() {
		buf := make([]byte, 1024)
		_, addr, err := pc.ReadFrom(buf)
		if err != nil {
			return
		}
		_, _ = pc.WriteTo([]byte("one"), addr)
		_, _ = pc.WriteTo([]byte("two"), addr)
	}()

	target := "udp://" + pc.LocalAddr().String()
	h, id := debugStoreHandler(t, "udp", "POST", target, DebugConfig{
		EnableServerSide: true, AllowPrivateNetworks: true, TimeoutMS: 2000,
	})
	data := debugSendData(t, h, id, `{"payload":"x","fields":{"datagrams":"2","read_ms":"1000"}}`)
	if data["ok"] != true || data["headers"].(map[string]any)["x-datagrams"] != "2" {
		t.Fatalf("udp datagrams: %+v", data)
	}
	var body struct {
		Datagrams []struct {
			Encoding string `json:"encoding"`
			Data     string `json:"data"`
		} `json:"datagrams"`
	}
	if err := json.Unmarshal([]byte(data["body"].(string)), &body); err != nil {
		t.Fatalf("datagram body: %v", err)
	}
	if len(body.Datagrams) != 2 || body.Datagrams[0].Encoding != "base64" || body.Datagrams[1].Data != "dHdv" {
		t.Fatalf("datagram payload: %+v", body)
	}
}

func TestRelayRawUsesTransportOpt(t *testing.T) {
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer pc.Close()
	go func() {
		buf := make([]byte, 1024)
		n, addr, err := pc.ReadFrom(buf)
		if err != nil {
			return
		}
		_, _ = pc.WriteTo(buf[:n], addr)
	}()

	target := "udp://" + pc.LocalAddr().String()
	h, id := debugStoreHandler(t, "raw", "POST", target, DebugConfig{
		EnableServerSide: true, AllowPrivateNetworks: true, TimeoutMS: 2000,
	}, map[string]string{"transport": "udp"})
	data := debugSendData(t, h, id, `{"payload":"raw-udp","fields":{"read_ms":"500"}}`)
	if data["ok"] != true || data["body"] != "raw-udp" {
		t.Fatalf("raw udp echo: %+v", data)
	}
}

func TestRelayUnixSocket(t *testing.T) {
	path := filepath.Join(t.TempDir(), "relay.sock")
	ln, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		buf := make([]byte, 2)
		if _, err := conn.Read(buf); err == nil {
			_, _ = conn.Write([]byte("ok"))
		}
	}()

	disabled, id := debugStoreHandler(t, "unix", "POST", path, DebugConfig{
		EnableServerSide: true, AllowPrivateNetworks: true, TimeoutMS: 2000,
	})
	if data := debugSendData(t, disabled, id, `{"payload":"hi","fields":{"expect_bytes":"2"}}`); data["ok"] != false ||
		!strings.Contains(data["error"].(string), "unix sockets are disabled") {
		t.Fatalf("unix must be gated: %+v", data)
	}

	enabled, id := debugStoreHandler(t, "unix", "POST", path, DebugConfig{
		EnableServerSide: true, AllowUnixSockets: true, TimeoutMS: 2000,
	})
	// The disabled attempt did not connect, so the listener still has capacity.
	data := debugSendData(t, enabled, id, `{"payload":"hi","fields":{"expect_bytes":"2"}}`)
	if data["ok"] != true || data["body"] != "ok" {
		t.Fatalf("unix echo: %+v", data)
	}
}

func TestRelaySocketSSRFAndCaps(t *testing.T) {
	h, id := debugStoreHandler(t, "tcp", "POST", "127.0.0.1:9", DebugConfig{
		EnableServerSide: true, TimeoutMS: 500,
	})
	data := debugSendData(t, h, id, `{"payload":"x"}`)
	if data["ok"] != false || !strings.Contains(data["error"].(string), "private") {
		t.Fatalf("loopback socket must be blocked: %+v", data)
	}

	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer pc.Close()
	small, smallID := debugStoreHandler(t, "udp", "POST", "udp://"+pc.LocalAddr().String(), DebugConfig{
		EnableServerSide: true, AllowPrivateNetworks: true, MaxBytes: 3,
	})
	if data := debugSendData(t, small, smallID, `{"payload":"hello"}`); data["ok"] != false {
		t.Fatalf("request body cap: %+v", data)
	}
}

func TestRelaySocketStream(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		buf := make([]byte, 4)
		_, _ = conn.Read(buf)
		_, _ = conn.Write([]byte("one"))
		time.Sleep(100 * time.Millisecond)
		_, _ = conn.Write([]byte("two"))
		time.Sleep(300 * time.Millisecond)
	}()

	target := "tcp://" + ln.Addr().String()
	h, id := debugStoreHandler(t, "tcp", "POST", target, DebugConfig{
		EnableServerSide: true, AllowPrivateNetworks: true, TimeoutMS: 2000,
	})
	start := debugStartData(t, h, id, `{"payload":"ping","fields":{"read_ms":"50"}}`)
	if start["ok"] != true {
		t.Fatalf("start: %+v", start)
	}
	sessionID, _ := start["sessionId"].(string)
	if sessionID == "" {
		t.Fatal("missing sessionId")
	}

	frames := []string{}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && len(frames) < 2 {
		rec := do(t, h, "POST", "/ui/v1/debugPoll", `{"sessionId":"`+sessionID+`","appKey":"default|v1"}`, "")
		data := decodeData(t, rec)
		messages, _ := data["messages"].([]any)
		for _, item := range messages {
			message := item.(map[string]any)
			if message["type"] == "frame" {
				frames = append(frames, message["body"].(string))
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	if len(frames) < 2 || frames[0] != "one" || frames[1] != "two" {
		t.Fatalf("stream frames: %+v", frames)
	}

	for i := 0; i < 2; i++ {
		rec := do(t, h, "POST", "/ui/v1/debugStop", `{"sessionId":"`+sessionID+`","appKey":"default|v1"}`, "")
		data := decodeData(t, rec)
		if data["ok"] != true {
			t.Fatalf("stop %d: %+v", i, data)
		}
	}
}

func TestRelaySocketReadTimeout(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		time.Sleep(500 * time.Millisecond)
	}()

	target := "tcp://" + ln.Addr().String()
	h, id := debugStoreHandler(t, "tcp", "POST", target, DebugConfig{
		EnableServerSide: true, AllowPrivateNetworks: true, TimeoutMS: 2000,
	})
	started := time.Now()
	data := debugSendData(t, h, id, `{"fields":{"read_ms":"100"}}`)
	if data["ok"] != true || data["body"] != "" {
		t.Fatalf("empty read window: %+v", data)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("read window not honoured: %v", elapsed)
	}
}

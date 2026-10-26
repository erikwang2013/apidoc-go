//go:build apidoc_debug_iot || apidoc_debug_all

package debug

import (
	"encoding/hex"
	"encoding/json"
	"net"
	"strings"
	"testing"
	"time"
)

func coapTestConfig() Config {
	return Config{AllowPrivateNetworks: true, TimeoutMS: 2000}
}

func coapTestRequest(target string, opts map[string]string) *Request {
	return &Request{Kind: "coap", Target: target, Opts: opts, TimeoutMS: 2000}
}

func coapTestBody(t *testing.T, response *Response) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal([]byte(response.Body.Data), &body); err != nil {
		t.Fatalf("body %q: %v", response.Body.Data, err)
	}
	return body
}

func TestCoAPGetRoundtrip(t *testing.T) {
	server, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	address := server.LocalAddr().String()
	done := make(chan error, 1)
	go func() {
		buf := make([]byte, 2048)
		n, peer, err := server.ReadFrom(buf)
		if err != nil {
			done <- err
			return
		}
		request, err := coapDecode(buf[:n])
		if err != nil {
			done <- err
			return
		}
		if request.packetType != coapTypeConfirmable || request.code != 1 {
			done <- errString("unexpected request type/code")
			return
		}
		if strings.Join(coapOptionStrings(request, 11), "/") != "sensors/temp" {
			done <- errString("unexpected path")
			return
		}
		if len(request.options) == 0 {
			done <- errString("expected content-format option")
			return
		}
		response := coapEncode(coapTypeAcknowledgement, 69, request.messageID, request.token,
			[]coapOption{{number: 12, value: []byte{0}}}, []byte("22.5"))
		_, err = server.WriteTo(response, peer)
		done <- err
	}()

	response, err := executeCoAP(coapTestConfig(), coapTestRequest("coap://"+address,
		map[string]string{"path": "/sensors/temp"}))
	if err != nil {
		t.Fatal(err)
	}
	if response.Headers["x-peer"] != address {
		t.Fatalf("x-peer = %q", response.Headers["x-peer"])
	}
	body := coapTestBody(t, response)
	if body["code"] != "2.05" {
		t.Fatalf("code = %v", body["code"])
	}
	payload := body["payload"].(map[string]any)
	if payload["encoding"] != "utf8" || payload["data"] != "22.5" {
		t.Fatalf("payload = %#v", payload)
	}
	if body["options"].(map[string]any)["content-format"] != "0" {
		t.Fatalf("options = %#v", body["options"])
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestCoAPObserveCollectsNotifications(t *testing.T) {
	server, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	address := server.LocalAddr().String()
	done := make(chan error, 1)
	go func() {
		buf := make([]byte, 2048)
		n, peer, err := server.ReadFrom(buf)
		if err != nil {
			done <- err
			return
		}
		request, err := coapDecode(buf[:n])
		if err != nil {
			done <- err
			return
		}
		if !coapHasObserve(buf[:n]) {
			done <- errString("observe option missing")
			return
		}
		first := coapEncode(coapTypeAcknowledgement, 69, request.messageID, request.token,
			[]coapOption{{number: 6, value: []byte{1}}}, []byte("one"))
		second := coapEncode(1, 69, request.messageID+1, request.token,
			[]coapOption{{number: 6, value: []byte{2}}}, []byte("two"))
		if _, err := server.WriteTo(first, peer); err != nil {
			done <- err
			return
		}
		_, err = server.WriteTo(second, peer)
		done <- err
	}()

	response, err := executeCoAP(coapTestConfig(), coapTestRequest("coap://"+address,
		map[string]string{"path": "/obs", "observe": "true", "messages": "2"}))
	if err != nil {
		t.Fatal(err)
	}
	body := coapTestBody(t, response)
	notifications, ok := body["notifications"].([]any)
	if !ok || len(notifications) != 2 {
		t.Fatalf("notifications = %#v", body)
	}
	if notifications[0].(map[string]any)["payload"].(map[string]any)["data"] != "one" ||
		notifications[1].(map[string]any)["payload"].(map[string]any)["data"] != "two" {
		t.Fatalf("notifications = %#v", notifications)
	}
	if notifications[1].(map[string]any)["options"].(map[string]any)["observe"] != "2" {
		t.Fatalf("second observe option = %#v", notifications[1])
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestCoAPTimeoutRSTAndCap(t *testing.T) {
	t.Run("timeout", func(t *testing.T) {
		server, err := net.ListenPacket("udp4", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		defer server.Close()
		started := time.Now()
		_, err = executeCoAP(coapTestConfig(), coapTestRequest("coap://"+server.LocalAddr().String(),
			map[string]string{"duration_ms": "50"}))
		if err == nil || !strings.Contains(err.Error(), "timed out") {
			t.Fatalf("timeout error = %v", err)
		}
		if time.Since(started) > 5*time.Second {
			t.Fatalf("timeout not bounded: %v", time.Since(started))
		}
	})

	t.Run("reset", func(t *testing.T) {
		server, err := net.ListenPacket("udp4", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		defer server.Close()
		go func() {
			buf := make([]byte, 2048)
			n, peer, err := server.ReadFrom(buf)
			if err != nil {
				return
			}
			request, err := coapDecode(buf[:n])
			if err != nil {
				return
			}
			_, _ = server.WriteTo(coapEncode(coapTypeReset, 0, request.messageID, request.token, nil, nil), peer)
		}()
		_, err = executeCoAP(coapTestConfig(), coapTestRequest("coap://"+server.LocalAddr().String(),
			map[string]string{"duration_ms": "500"}))
		if err == nil || !strings.Contains(err.Error(), "RST") {
			t.Fatalf("reset error = %v", err)
		}
	})

	t.Run("cap", func(t *testing.T) {
		server, err := net.ListenPacket("udp4", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		defer server.Close()
		go func() {
			buf := make([]byte, 2048)
			n, peer, err := server.ReadFrom(buf)
			if err != nil {
				return
			}
			request, err := coapDecode(buf[:n])
			if err != nil {
				return
			}
			_, _ = server.WriteTo(coapEncode(coapTypeAcknowledgement, 69, request.messageID,
				request.token, nil, make([]byte, 80)), peer)
		}()
		cfg := coapTestConfig()
		cfg.MaxBytes = 40
		_, err = executeCoAP(cfg, coapTestRequest("coap://"+server.LocalAddr().String(),
			map[string]string{"duration_ms": "500"}))
		if err == nil || !strings.Contains(err.Error(), "exceeds max_bytes") {
			t.Fatalf("cap error = %v", err)
		}
	})
}

func TestCoAPRejectsBadTargetsAndIgnoresGarbage(t *testing.T) {
	if _, err := executeCoAP(coapTestConfig(), coapTestRequest("coaps://127.0.0.1:5683", nil)); err == nil ||
		!strings.Contains(err.Error(), "coaps") {
		t.Fatalf("coaps error = %v", err)
	}
	if _, err := executeCoAP(coapTestConfig(), coapTestRequest("coap://127.0.0.1:5683",
		map[string]string{"method": "PATCH"})); err == nil ||
		!strings.Contains(err.Error(), "unsupported coap method") {
		t.Fatalf("method error = %v", err)
	}
	if _, err := executeCoAP(Config{}, coapTestRequest("coap://127.0.0.1:5683", nil)); err == nil ||
		!strings.Contains(err.Error(), "private/loopback") {
		t.Fatalf("guard error = %v", err)
	}

	server, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	go func() {
		buf := make([]byte, 2048)
		n, peer, err := server.ReadFrom(buf)
		if err != nil {
			return
		}
		request, err := coapDecode(buf[:n])
		if err != nil {
			return
		}
		_, _ = server.WriteTo([]byte{0xff, 0x00}, peer) // non-CoAP datagram
		_, _ = server.WriteTo(coapEncode(coapTypeAcknowledgement, 69, request.messageID,
			request.token, nil, []byte("ok")), peer)
	}()
	response, err := executeCoAP(coapTestConfig(), coapTestRequest("coap://"+server.LocalAddr().String(),
		map[string]string{"duration_ms": "500"}))
	if err != nil || coapTestBody(t, response)["code"] != "2.05" {
		t.Fatalf("garbage handling: %v", err)
	}
}

func TestCoAPCodecGolden(t *testing.T) {
	request, err := coapBuildRequest("GET", "/sensors/temp", nil, nil, 0, false, 0x1234, []byte{1, 2})
	if err != nil {
		t.Fatal(err)
	}
	want := "420112340102b773656e736f72730474656d7010"
	if hex.EncodeToString(request) != want {
		t.Fatalf("request = %x, want %s", request, want)
	}
	packet, err := coapDecode(request)
	if err != nil {
		t.Fatal(err)
	}
	if packet.packetType != coapTypeConfirmable || packet.code != 1 || packet.messageID != 0x1234 ||
		len(packet.token) != 2 || strings.Join(coapOptionStrings(packet, 11), "/") != "sensors/temp" {
		t.Fatalf("decoded = %#v", packet)
	}
	if coapCodeString(69) != "2.05" || coapCodeString(132) != "4.04" {
		t.Fatalf("code strings: %s %s", coapCodeString(69), coapCodeString(132))
	}
}

type errString string

func (e errString) Error() string { return string(e) }

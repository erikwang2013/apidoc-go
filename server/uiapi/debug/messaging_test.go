//go:build apidoc_debug_messaging || apidoc_debug_mqtt || apidoc_debug_stomp || apidoc_debug_amqp || apidoc_debug_kafka || apidoc_debug_xmpp || apidoc_debug_all

package debug

import (
	"encoding/json"
	"net"
	"strings"
	"testing"
	"time"
)

func messagingConfig() Config {
	return Config{EnableServerSide: true, AllowPrivateNetworks: true, TimeoutMS: 1500, MaxBytes: 1 << 20}
}

func listenLocal(t *testing.T) net.Listener {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	return ln
}

func TestMessagingTargetParsing(t *testing.T) {
	host, port, scheme, err := parseMessagingTarget("mqtt://broker.test:1884", "mqtt", "1883", map[string]bool{"mqtt": true})
	if err != nil || host != "broker.test" || port != "1884" || scheme != "mqtt" {
		t.Fatalf("target = %q %q %q err=%v", host, port, scheme, err)
	}
	host, port, _, err = parseMessagingTarget("broker.test", "mqtt", "1883", map[string]bool{"mqtt": true})
	if err != nil || host != "broker.test" || port != "1883" {
		t.Fatalf("default target = %q %q err=%v", host, port, err)
	}
	for _, bad := range []string{
		"mqtt://user:pw@broker.test",
		"mqtt://broker.test/path",
		"mqtt://broker.test?x=1",
		"amqp://broker.test:5672",
	} {
		if _, _, _, err := parseMessagingTarget(bad, "mqtt", "1883", map[string]bool{"mqtt": true}); err == nil {
			t.Fatalf("target %q was accepted", bad)
		}
	}
}

func TestMessagingLimits(t *testing.T) {
	cfg := Config{TimeoutMS: 500}
	req := &Request{Opts: map[string]string{"duration_ms": "999999", "messages": "5000"}}
	if got := windowDuration(cfg, req); got != 500*time.Millisecond {
		t.Fatalf("window = %v", got)
	}
	if got := wantMessages(req); got != 1000 {
		t.Fatalf("messages = %d", got)
	}
	req.Opts = map[string]string{"duration_ms": "bad", "messages": "-3"}
	if got := windowDuration(cfg, req); got != 500*time.Millisecond {
		t.Fatalf("default window = %v", got)
	}
	if got := wantMessages(req); got != 1 {
		t.Fatalf("default messages = %d", got)
	}
	total := 0
	if err := addMessageBytes(&total, 10, 20); err != nil {
		t.Fatal(err)
	}
	if err := addMessageBytes(&total, 20, 20); err == nil || !strings.Contains(err.Error(), "max_bytes") {
		t.Fatalf("cap err = %v", err)
	}
	if _, err := optBool(&Request{Opts: map[string]string{"consume": "yes"}}, "consume", false); err == nil {
		t.Fatal("invalid bool was accepted")
	}
	if got, err := optBool(&Request{Opts: map[string]string{}}, "consume", true); err != nil || !got {
		t.Fatalf("default bool = %v err=%v", got, err)
	}
}

func TestMessagingRedaction(t *testing.T) {
	if got := redactSecrets("auth pw-secret failed", "pw-secret"); strings.Contains(got, "pw-secret") || !strings.Contains(got, "***") {
		t.Fatalf("redacted = %q", got)
	}
	if err := redactedError("password pw", "pw"); err == nil || err.Error() != "password ***" {
		t.Fatalf("redacted error = %v", err)
	}
}

func TestMessagingUncompiledFamilyFailsClosed(t *testing.T) {
	_, err := Execute(messagingConfig(), &Request{Kind: "not-a-family"})
	if err == nil || !strings.Contains(err.Error(), "not compiled") {
		t.Fatalf("execute err = %v", err)
	}
	_, err = OpenStream(messagingConfig(), &Request{Kind: "not-a-family"})
	if err == nil || !strings.Contains(err.Error(), "not compiled") {
		t.Fatalf("stream err = %v", err)
	}
}

// decodeMessagingJSON decodes an executor response body.
func decodeMessagingJSON(t *testing.T, resp *Response) map[string]any {
	t.Helper()
	value := map[string]any{}
	if err := json.Unmarshal([]byte(resp.Body.Data), &value); err != nil {
		t.Fatalf("body %q: %v", resp.Body.Data, err)
	}
	return value
}

func messagingPayload(message map[string]any) map[string]any {
	payload, _ := message["payload"].(map[string]any)
	return payload
}

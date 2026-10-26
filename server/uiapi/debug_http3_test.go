//go:build apidoc_debug_http3 || apidoc_debug_all

package uiapi

import (
	"strings"
	"testing"
)

func TestRelayHTTP3WiredAndFailClosed(t *testing.T) {
	// A registered executor rejects a non-TLS target with a protocol error, not
	// the "not compiled" dispatcher error.
	h, id := debugStoreHandler(t, "http3", "GET", "http://127.0.0.1:9",
		DebugConfig{EnableServerSide: true, AllowPrivateNetworks: true, TimeoutMS: 300})
	data := debugSendData(t, h, id, `{"payload":""}`)
	if data["ok"] != false || !strings.Contains(data["error"].(string), "must use https") {
		t.Fatalf("http3 scheme: %+v", data)
	}

	// No server on this UDP port: the handshake fails honestly instead of
	// reporting success.
	h, id = debugStoreHandler(t, "http3", "GET", "https://127.0.0.1:9",
		DebugConfig{EnableServerSide: true, AllowPrivateNetworks: true, TimeoutMS: 300})
	data = debugSendData(t, h, id, `{"payload":""}`)
	if data["ok"] != false || !strings.Contains(data["error"].(string), "http3 request failed") {
		t.Fatalf("http3 failure: %+v", data)
	}

	// SSRF is checked before any QUIC dial.
	h, id = debugStoreHandler(t, "http3", "GET", "https://127.0.0.1:9",
		DebugConfig{EnableServerSide: true, TimeoutMS: 300})
	data = debugSendData(t, h, id, `{"payload":""}`)
	if data["ok"] != false || !strings.Contains(data["error"].(string), "private") {
		t.Fatalf("http3 ssrf: %+v", data)
	}
}

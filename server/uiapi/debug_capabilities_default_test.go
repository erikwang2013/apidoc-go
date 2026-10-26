//go:build !apidoc_debug_http && !apidoc_debug_sockets && !apidoc_debug_rpc && !apidoc_debug_grpc && !apidoc_debug_http3 && !apidoc_debug_messaging && !apidoc_debug_mqtt && !apidoc_debug_stomp && !apidoc_debug_amqp && !apidoc_debug_kafka && !apidoc_debug_xmpp && !apidoc_debug_all

package uiapi

import (
	"reflect"
	"testing"
)

func TestDebugCapabilitiesDefaultHasNoRelayModes(t *testing.T) {
	h := newHandlerWith(t, Options{})
	debug := decodeData(t, do(t, h, "GET", "/ui/v1/capabilities", "", ""))["features"].(map[string]any)["debug"].(map[string]any)
	want := []any{"browser-http", "browser-websocket", "browser-sse"}
	if !reflect.DeepEqual(debug["modes"], want) {
		t.Fatalf("modes = %#v", debug["modes"])
	}
	if relay, _ := debug["relay"].([]any); len(relay) != 0 {
		t.Fatalf("relay = %#v", debug["relay"])
	}
}

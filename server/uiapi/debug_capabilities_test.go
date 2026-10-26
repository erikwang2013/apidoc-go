//go:build apidoc_debug_all

package uiapi

import (
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/erikwang2013/apidoc-go/model"
	"github.com/erikwang2013/apidoc-go/store"
)

func TestDebugCapabilitiesTagged(t *testing.T) {
	h := newHandlerWith(t, Options{})
	features := decodeData(t, do(t, h, "GET", "/ui/v1/capabilities", "", ""))["features"].(map[string]any)
	debug := features["debug"].(map[string]any)
	wantRelay := []any{
		"http", "http2", "soap", "graphql", "jsonrpc", "xmlrpc", "odata", "mcp", "a2a", "webhook",
		"tcp", "udp", "raw", "unix", "thrift", "dubbo", "hessian", "iso8583",
	}
	if !reflect.DeepEqual(debug["relay"], wantRelay) {
		t.Fatalf("relay = %#v", debug["relay"])
	}
	wantModes := []any{"browser-http", "browser-websocket", "browser-sse"}
	if !reflect.DeepEqual(debug["modes"], wantModes) {
		t.Fatalf("modes = %#v", debug["modes"])
	}
}

func debugDetailStore() (*store.Store, string, string) {
	st := store.New("Debug")
	st.SetAppMeta("default", "Default")
	st.Register("POST", "tcp://example.com:9000", model.Doc{
		App: "default", Version: "v1", Controller: "debug", Title: "TCP",
		Protocol:     "tcp",
		ProtocolOpts: map[string]string{"target": "tcp://example.com:9000"},
	})
	st.Register("POST", "grpc://example.com:443", model.Doc{
		App: "default", Version: "v1", Controller: "debug", Title: "gRPC",
		Protocol:     "grpc",
		ProtocolOpts: map[string]string{"target": "grpc://example.com:443"},
	})
	return st, store.ActionID("default", "v1", "POST", "tcp://example.com:9000"),
		store.ActionID("default", "v1", "POST", "grpc://example.com:443")
}

func debugDetail(t *testing.T, h http.Handler, path string) map[string]any {
	t.Helper()
	rec := do(t, h, "POST", "/ui/v1/detail", `{"path":"`+path+`"}`, "")
	return decodeData(t, rec)
}

func TestDebugDetailRelayModesTagged(t *testing.T) {
	st, tcpID, grpcID := debugDetailStore()
	enabled := newHandlerWith(t, Options{Store: st, Debug: DebugConfig{EnableServerSide: true}})

	data := debugDetail(t, enabled, tcpID)
	protocol := data["protocol"].(map[string]any)
	if protocol["mode"] != "relay-unary" || protocol["debuggable"] != true ||
		protocol["http"] != false || protocol["debugReason"] != "" {
		t.Fatalf("tcp protocol: %+v", protocol)
	}
	debug := data["debug"].(map[string]any)
	if debug["relayEnabled"] != true {
		t.Fatalf("tcp debug: %+v", debug)
	}
	foundTarget := false
	for _, item := range debug["fields"].([]any) {
		if field := item.(map[string]any); field["name"] == "target" {
			foundTarget = true
		}
	}
	if !foundTarget {
		t.Fatalf("tcp fields: %+v", debug["fields"])
	}

	data = debugDetail(t, enabled, grpcID)
	protocol = data["protocol"].(map[string]any)
	if protocol["mode"] != "relay-unary" || protocol["debuggable"] != false ||
		!strings.Contains(protocol["debugReason"].(string), "not compiled") {
		t.Fatalf("grpc protocol: %+v", protocol)
	}

	disabled := newHandlerWith(t, Options{Store: st})
	data = debugDetail(t, disabled, tcpID)
	protocol = data["protocol"].(map[string]any)
	if protocol["debuggable"] != false || protocol["debugReason"] != "server-side debug is disabled" {
		t.Fatalf("disabled tcp: %+v", protocol)
	}
}

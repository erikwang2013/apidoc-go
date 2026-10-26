//go:build apidoc_debug_all

package uiapi

import (
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/erikwang2013/apidoc-go/model"
	"github.com/erikwang2013/apidoc-go/server/uiapi/debug"
	"github.com/erikwang2013/apidoc-go/store"
)

func TestDebugCapabilitiesTagged(t *testing.T) {
	h := newHandlerWith(t, Options{})
	features := decodeData(t, do(t, h, "GET", "/ui/v1/capabilities", "", ""))["features"].(map[string]any)
	debug := features["debug"].(map[string]any)
	wantRelay := []any{
		"http", "http2", "http3", "soap", "graphql", "jsonrpc", "xmlrpc", "odata", "mcp", "a2a", "webhook",
		"tcp", "udp", "raw", "unix", "mqtt", "stomp", "amqp", "kafka", "xmpp",
		"coap", "lwm2m", "modbus", "dnp3", "iec104", "bacnet", "opcua",
		"rtsp", "sip", "rtp", "rtcp", "rtmp",
		"grpc", "thrift", "dubbo", "hessian", "iso8583",
	}
	if !reflect.DeepEqual(debug["relay"], wantRelay) {
		t.Fatalf("relay = %#v", debug["relay"])
	}
	wantModes := []any{"browser-http", "browser-websocket", "browser-sse", "relay-unary", "relay-stream"}
	if !reflect.DeepEqual(debug["modes"], wantModes) {
		t.Fatalf("modes = %#v", debug["modes"])
	}
}

func debugDetailStore() (*store.Store, string, string, string) {
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
	st.Register("POST", "modbus://example.com:502", model.Doc{
		App: "default", Version: "v1", Controller: "debug", Title: "Modbus",
		Protocol:     "modbus",
		ProtocolOpts: map[string]string{"target": "modbus://example.com:502"},
	})
	return st, store.ActionID("default", "v1", "POST", "tcp://example.com:9000"),
		store.ActionID("default", "v1", "POST", "grpc://example.com:443"),
		store.ActionID("default", "v1", "POST", "modbus://example.com:502")
}

func debugDetail(t *testing.T, h http.Handler, path string) map[string]any {
	t.Helper()
	rec := do(t, h, "POST", "/ui/v1/detail", `{"path":"`+path+`"}`, "")
	return decodeData(t, rec)
}

func TestDebugDetailRelayModesTagged(t *testing.T) {
	st, tcpID, grpcID, modbusID := debugDetailStore()
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
	if protocol["mode"] != "relay-unary" || protocol["debuggable"] != true || protocol["debugReason"] != "" {
		t.Fatalf("grpc protocol: %+v", protocol)
	}

	data = debugDetail(t, enabled, modbusID)
	protocol = data["protocol"].(map[string]any)
	if protocol["mode"] != "relay-unary" || protocol["debuggable"] != true ||
		protocol["debugReason"] != "" {
		t.Fatalf("modbus protocol: %+v", protocol)
	}

	rtspTarget := "rtsp://example.com:554/live"
	st.Register("POST", rtspTarget, model.Doc{
		App: "default", Version: "v1", Controller: "debug", Title: "RTSP",
		Protocol: "rtsp", ProtocolOpts: map[string]string{"target": rtspTarget},
	})
	rtspID := store.ActionID("default", "v1", "POST", rtspTarget)
	protocol = debugDetail(t, enabled, rtspID)["protocol"].(map[string]any)
	if protocol["mode"] != "relay-unary" || protocol["debuggable"] != true ||
		protocol["debugReason"] != "" {
		t.Fatalf("rtsp protocol: %+v", protocol)
	}

	disabled := newHandlerWith(t, Options{Store: st})
	data = debugDetail(t, disabled, tcpID)
	protocol = data["protocol"].(map[string]any)
	if protocol["debuggable"] != false || protocol["debugReason"] != "server-side debug is disabled" {
		t.Fatalf("disabled tcp: %+v", protocol)
	}
	protocol = debugDetail(t, disabled, rtspID)["protocol"].(map[string]any)
	if protocol["debuggable"] != false || protocol["debugReason"] != "server-side debug is disabled" {
		t.Fatalf("disabled rtsp: %+v", protocol)
	}
}

func TestDebugDetailMessagingModesTagged(t *testing.T) {
	st := store.New("Debug")
	st.SetAppMeta("default", "Default")
	ids := map[string]string{}
	for _, entry := range [][2]string{
		{"mqtt", "PUBLISH"}, {"mqtt", "SUBSCRIBE"},
		{"stomp", "SEND"}, {"stomp", "SUBSCRIBE"},
		{"amqp", "PUBLISH"}, {"amqp", "CONSUME"}, {"kafka", "CONSUME"}, {"xmpp", "MESSAGE"},
	} {
		protocol, method := entry[0], entry[1]
		target := protocol + "://example.com:9000/" + strings.ToLower(method)
		st.Register(method, target, model.Doc{
			App: "default", Version: "v1", Controller: "debug", Title: protocol + " " + method,
			Protocol:     protocol,
			ProtocolOpts: map[string]string{"target": protocol + "://example.com:9000"},
		})
		ids[protocol+"/"+method] = store.ActionID("default", "v1", method, target)
	}
	h := newHandlerWith(t, Options{Store: st, Debug: DebugConfig{EnableServerSide: true}})
	want := map[string]string{
		"mqtt/PUBLISH":    "relay-unary",
		"mqtt/SUBSCRIBE":  "relay-stream",
		"stomp/SEND":      "relay-unary",
		"stomp/SUBSCRIBE": "relay-stream",
		"amqp/PUBLISH":    "relay-unary",
		"amqp/CONSUME":    "relay-stream",
		"kafka/CONSUME":   "relay-stream",
		"xmpp/MESSAGE":    "relay-stream",
	}
	for key, mode := range want {
		protocol := debugDetail(t, h, ids[key])["protocol"].(map[string]any)
		if protocol["mode"] != mode || protocol["debuggable"] != true || protocol["debugReason"] != "" {
			t.Fatalf("%s protocol: %+v", key, protocol)
		}
		// Every advertised mode must have a compiled implementation: a stream
		// action without an opener (or a unary action without an executor)
		// would return ok:false from debugStart/debugSend.
		kind, _, _ := strings.Cut(key, "/")
		if mode == "relay-stream" && !debug.HasStream(kind) {
			t.Fatalf("%s advertises relay-stream without a compiled stream", key)
		}
		if mode == "relay-unary" && !debug.HasExecutor(kind) {
			t.Fatalf("%s advertises relay-unary without a compiled executor", key)
		}
	}
	disabled := newHandlerWith(t, Options{Store: st})
	protocol := debugDetail(t, disabled, ids["mqtt/PUBLISH"])["protocol"].(map[string]any)
	if protocol["mode"] != "relay-unary" || protocol["debuggable"] != false ||
		protocol["debugReason"] != "server-side debug is disabled" {
		t.Fatalf("disabled mqtt: %+v", protocol)
	}
}

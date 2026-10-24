package model

import "testing"

func TestProtocolOf(t *testing.T) {
	cases := []struct{ in, want string }{
		{"https://api.example.com/x", "http"},
		{"wss://api.example.com/ws", "ws"},
		{"grpcs://host/pkg.Svc/M", "grpc"},
		{"mqtts://broker:8883/topic", "mqtt"},
		{"amqps://broker/queue", "amqp"},
		{"h2://host", "http2"},
		{"h3://host", "http3"},
		{"kafka://broker/topic", "kafka"},
		{"dubbo://host/svc", "dubbo"},
		{"thrift://host/svc", "thrift"},
		{"iso8583://host:9000/0100", "iso8583"},
		{"opc.tcp://host:4840", "opcua"},
		{"modbus://host:502", "modbus"},
		{"iec104://host:2404", "iec104"},
		{"bacnet://host", "bacnet"},
		{"webrtc://host", "webrtc"},
		{"socketio://host", "socketio"},
		{"rtsp://host", "rtsp"},
		{"rtmp://host", "rtmp"},
		{"sip://host", "sip"},
		{"xmpp://host", "xmpp"},
		{"unix:///tmp/x.sock", "unix"},
		{"raw://host", "rawsocket"},
		{"json-rpc://host", "jsonrpc"},
		{"websocket://host", "ws"},
		{"rest://host", "http"},
		{"tcp://127.0.0.1:9000", "tcp"},
		{"ssh://host", "ssh"},
		{"/api/users", "http"},
		{"localhost:50051", "http"}, // no "://": not a scheme
		{"", "http"},
	}
	for _, c := range cases {
		if got := ProtocolOf(c.in); got != c.want {
			t.Errorf("ProtocolOf(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestNormalizeProtocol(t *testing.T) {
	cases := []struct{ in, want string }{
		{"REST", "http"},
		{"HTTP/2", "http2"},
		{"HTTP/3", "http3"},
		{"SOAP", "soap"},
		{"GraphQL", "graphql"},
		{"JSON-RPC", "jsonrpc"},
		{"XML-RPC", "xmlrpc"},
		{"OData", "odata"},
		{"Webhooks", "webhook"},
		{"gRPC", "grpc"},
		{"Dubbo", "dubbo"},
		{"Thrift", "thrift"},
		{"Hessian", "hessian"},
		{"MCP", "mcp"},
		{"A2A", "a2a"},
		{"ISO 8583", "iso8583"},
		{"ISO  8583", "iso8583"},
		{"WebSocket", "ws"},
		{"SSE", "sse"},
		{"Server-Sent Events", "sse"},
		{"Socket.IO", "socketio"},
		{"WebRTC DataChannel", "webrtc"},
		{"RTSP", "rtsp"},
		{"RTMP", "rtmp"},
		{"HLS", "hls"},
		{"HTTP Live Streaming", "hls"},
		{"SIP", "sip"},
		{"XMPP", "xmpp"},
		{"AMQP", "amqp"},
		{"AMQP 0-9-1", "amqp"},
		{"MQTT", "mqtt"},
		{"MQTT 5", "mqtt"},
		{"STOMP", "stomp"},
		{"STOMP 1.2", "stomp"},
		{"Kafka", "kafka"},
		{"CoAP", "coap"},
		{"LwM2M", "lwm2m"},
		{"OPC UA", "opcua"},
		{"OPC_UA", "opcua"},
		{"Modbus TCP", "modbus"},
		{"DNP3", "dnp3"},
		{"IEC 60870-5-104", "iec104"},
		{"BACnet/IP", "bacnet"},
		{"RTP", "rtp"},
		{"RTCP", "rtcp"},
		{"TCP", "tcp"},
		{"UDP", "udp"},
		{"Raw Socket", "rawsocket"},
		{"Unix Domain Socket", "unix"},
		{"HTTP/HTTPS Binary Body", "http"},
		{"WebSocket Binary Frame", "ws"},
		{"STOMP Binary Body", "stomp"},
		{"gRPC Binary Body", "grpc"},
		{"Kafka Binary Body", "kafka"},
		{"ISO 8583 Binary Body", "iso8583"},
		{"OPC UA Binary", "opcua"},
		{"Modbus TCP Binary", "modbus"},
		{"IEC 60870-5-104 Binary", "iec104"},
		{"BACnet/IP Binary", "bacnet"},
		{"custom", "custom"},
	}
	for _, c := range cases {
		if got := NormalizeProtocol(c.in); got != c.want {
			t.Errorf("NormalizeProtocol(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestProtocolsRegistry(t *testing.T) {
	list := Protocols()
	if len(list) < 40 {
		t.Fatalf("registry has %d protocols, want at least 40", len(list))
	}
	seen := map[string]bool{}
	for _, p := range list {
		if p.Key == "" || p.Name == "" || p.Category == "" {
			t.Errorf("incomplete protocol: %+v", p)
		}
		if seen[p.Key] {
			t.Errorf("duplicate protocol key %q", p.Key)
		}
		seen[p.Key] = true
		if got := NormalizeProtocol(p.Key); got != p.Key {
			t.Errorf("key %q normalizes to %q", p.Key, got)
		}
	}
	for _, key := range []string{
		"http", "http2", "http3", "soap", "graphql", "jsonrpc", "xmlrpc",
		"odata", "webhook", "grpc", "dubbo", "thrift", "hessian", "mcp",
		"a2a", "iso8583", "ws", "sse", "socketio", "webrtc", "rtsp",
		"rtmp", "hls", "sip", "xmpp", "amqp", "mqtt", "stomp", "kafka",
		"coap", "lwm2m", "opcua", "modbus", "dnp3", "iec104", "bacnet",
		"rtp", "rtcp", "tcp", "udp", "rawsocket", "unix",
	} {
		if !seen[key] {
			t.Errorf("registry missing %q", key)
		}
	}
}

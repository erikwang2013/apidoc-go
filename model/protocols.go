package model

import "strings"

// ProtocolInfo describes one protocol accepted by @protocol and exposed
// by /api/protocols for the external UI.
type ProtocolInfo struct {
	Key         string   `json:"key"`
	Name        string   `json:"name"`
	Category    string   `json:"category"`
	Schemes     []string `json:"schemes,omitempty"`
	Aliases     []string `json:"aliases,omitempty"`
	DefaultPort int      `json:"default_port,omitempty"`
}

// protocolList is the annotation registry. Keys are stable; aliases are
// accepted by @protocol and NormalizeProtocol. The library documents
// these protocols but does not implement them.
var protocolList = []ProtocolInfo{
	{Key: "http", Name: "HTTP/REST", Category: "HTTP", Schemes: []string{"http", "https", "rest"}, Aliases: []string{"rest", "restful", "http/1.1", "http1", "http 1.1", "http/https binary body", "http binary body", "https binary body"}, DefaultPort: 80},
	{Key: "http2", Name: "HTTP/2", Category: "HTTP", Schemes: []string{"http2", "h2", "h2c"}, Aliases: []string{"http/2", "http 2", "h2", "h2c", "http/2 binary body"}, DefaultPort: 443},
	{Key: "http3", Name: "HTTP/3", Category: "HTTP", Schemes: []string{"http3", "h3"}, Aliases: []string{"http/3", "http 3", "h3", "quic", "http/3 binary body"}, DefaultPort: 443},
	{Key: "soap", Name: "SOAP", Category: "RPC/HTTP", Schemes: []string{"soap", "soaps"}, Aliases: []string{"soap 1.1", "soap 1.2", "soap11", "soap12"}, DefaultPort: 80},
	{Key: "graphql", Name: "GraphQL", Category: "API", Schemes: []string{"graphql", "graphqls"}, DefaultPort: 80},
	{Key: "jsonrpc", Name: "JSON-RPC", Category: "RPC", Schemes: []string{"jsonrpc", "json-rpc"}, Aliases: []string{"json-rpc", "json rpc", "json-rpc 2.0", "jsonrpc2", "json rpc 2.0"}, DefaultPort: 80},
	{Key: "xmlrpc", Name: "XML-RPC", Category: "RPC", Schemes: []string{"xmlrpc", "xml-rpc"}, Aliases: []string{"xml-rpc", "xml rpc"}, DefaultPort: 80},
	{Key: "odata", Name: "OData", Category: "API", Schemes: []string{"odata", "odatas"}, DefaultPort: 80},
	{Key: "webhook", Name: "Webhooks", Category: "API", Schemes: []string{"webhook", "webhooks"}, Aliases: []string{"webhooks"}, DefaultPort: 80},
	{Key: "grpc", Name: "gRPC", Category: "RPC", Schemes: []string{"grpc", "grpcs"}, Aliases: []string{"grpc binary body"}, DefaultPort: 443},
	{Key: "dubbo", Name: "Dubbo", Category: "RPC", Schemes: []string{"dubbo", "dubbos"}, Aliases: []string{"dubbo binary body"}, DefaultPort: 20880},
	{Key: "thrift", Name: "Thrift", Category: "RPC", Schemes: []string{"thrift", "thrifts"}, Aliases: []string{"thrift binary body"}, DefaultPort: 9090},
	{Key: "hessian", Name: "Hessian", Category: "RPC", Schemes: []string{"hessian", "hessians"}, Aliases: []string{"hessian 2.0", "hessian binary body"}, DefaultPort: 80},
	{Key: "mcp", Name: "MCP", Category: "AI", Schemes: []string{"mcp"}, DefaultPort: 80},
	{Key: "a2a", Name: "A2A", Category: "AI", Schemes: []string{"a2a"}, DefaultPort: 80},
	{Key: "iso8583", Name: "ISO 8583", Category: "Financial", Schemes: []string{"iso8583"}, Aliases: []string{"iso 8583", "iso-8583", "iso8583 binary body", "iso 8583 binary body"}},
	{Key: "ws", Name: "WebSocket", Category: "Realtime", Schemes: []string{"ws", "wss", "websocket"}, Aliases: []string{"websocket", "websocket binary frame", "ws binary frame"}, DefaultPort: 80},
	{Key: "sse", Name: "SSE", Category: "Realtime", Schemes: []string{"sse", "sses"}, Aliases: []string{"server-sent events", "server sent events", "event-stream"}, DefaultPort: 80},
	{Key: "socketio", Name: "Socket.IO", Category: "Realtime", Schemes: []string{"socketio", "socket.io"}, Aliases: []string{"socket.io", "socket io"}, DefaultPort: 80},
	{Key: "webrtc", Name: "WebRTC DataChannel", Category: "Realtime", Schemes: []string{"webrtc", "webrtcs"}, Aliases: []string{"webrtc datachannel", "webrtc data channel", "webrtc datachannels"}, DefaultPort: 80},
	{Key: "rtsp", Name: "RTSP", Category: "Media", Schemes: []string{"rtsp", "rtsps"}, Aliases: []string{"real time streaming protocol"}, DefaultPort: 554},
	{Key: "rtmp", Name: "RTMP", Category: "Media", Schemes: []string{"rtmp", "rtmps"}, DefaultPort: 1935},
	{Key: "hls", Name: "HLS", Category: "Media", Schemes: []string{"hls"}, Aliases: []string{"http live streaming"}, DefaultPort: 80},
	{Key: "sip", Name: "SIP", Category: "Telecom", Schemes: []string{"sip", "sips"}, Aliases: []string{"session initiation protocol"}, DefaultPort: 5060},
	{Key: "xmpp", Name: "XMPP", Category: "Telecom", Schemes: []string{"xmpp", "xmpps"}, Aliases: []string{"extensible messaging and presence protocol"}, DefaultPort: 5222},
	{Key: "amqp", Name: "AMQP", Category: "Messaging", Schemes: []string{"amqp", "amqps"}, Aliases: []string{"amqp 0-9-1", "amqp 1.0", "amqp binary body"}, DefaultPort: 5672},
	{Key: "mqtt", Name: "MQTT", Category: "Messaging", Schemes: []string{"mqtt", "mqtts"}, Aliases: []string{"mqtt 3.1.1", "mqtt 5", "mqtt binary body"}, DefaultPort: 1883},
	{Key: "stomp", Name: "STOMP", Category: "Messaging", Schemes: []string{"stomp", "stomps"}, Aliases: []string{"stomp 1.2", "stomp binary body"}, DefaultPort: 61613},
	{Key: "kafka", Name: "Kafka", Category: "Messaging", Schemes: []string{"kafka", "kafkas"}, Aliases: []string{"kafka binary body"}, DefaultPort: 9092},
	{Key: "coap", Name: "CoAP", Category: "IoT", Schemes: []string{"coap", "coaps"}, Aliases: []string{"coap binary body"}, DefaultPort: 5683},
	{Key: "lwm2m", Name: "LwM2M", Category: "IoT", Schemes: []string{"lwm2m"}, Aliases: []string{"lightweight m2m"}, DefaultPort: 5683},
	{Key: "opcua", Name: "OPC UA", Category: "Industrial", Schemes: []string{"opcua", "opc.tcp"}, Aliases: []string{"opc ua", "opc-ua", "opc ua binary", "opcua binary"}, DefaultPort: 4840},
	{Key: "modbus", Name: "Modbus TCP", Category: "Industrial", Schemes: []string{"modbus", "modbus+tcp"}, Aliases: []string{"modbus tcp", "modbus-tcp", "modbus tcp binary"}, DefaultPort: 502},
	{Key: "dnp3", Name: "DNP3", Category: "Industrial", Schemes: []string{"dnp3"}, Aliases: []string{"distributed network protocol", "dnp3 binary"}, DefaultPort: 20000},
	{Key: "iec104", Name: "IEC 60870-5-104", Category: "Industrial", Schemes: []string{"iec104"}, Aliases: []string{"iec 60870-5-104", "iec-60870-5-104", "iec60870-5-104", "iec 60870 5 104", "iec104 binary", "iec 60870-5-104 binary", "iec 60870-5-104 binary body"}, DefaultPort: 2404},
	{Key: "bacnet", Name: "BACnet/IP", Category: "Industrial", Schemes: []string{"bacnet", "bacnet+ip"}, Aliases: []string{"bacnet/ip", "bacnet-ip", "bacnet ip", "bacnet binary", "bacnet/ip binary", "bacnet/ip binary body"}, DefaultPort: 47808},
	{Key: "rtp", Name: "RTP", Category: "Media", Schemes: []string{"rtp"}, Aliases: []string{"rtp binary"}},
	{Key: "rtcp", Name: "RTCP", Category: "Media", Schemes: []string{"rtcp"}},
	{Key: "tcp", Name: "TCP", Category: "Transport", Schemes: []string{"tcp"}, Aliases: []string{"tcp binary body"}},
	{Key: "udp", Name: "UDP", Category: "Transport", Schemes: []string{"udp"}, Aliases: []string{"udp binary body"}},
	{Key: "rawsocket", Name: "Raw Socket", Category: "Transport", Schemes: []string{"raw", "rawsocket"}, Aliases: []string{"raw socket", "raw-socket"}},
	{Key: "unix", Name: "Unix Domain Socket", Category: "Transport", Schemes: []string{"unix"}, Aliases: []string{"unix domain socket", "uds", "unix socket"}},
}

var (
	protocolByKey  = map[string]ProtocolInfo{}
	protocolScheme = map[string]string{}
	protocolAlias  = map[string]string{}
)

func init() {
	for _, p := range protocolList {
		protocolByKey[p.Key] = p
		protocolAlias[strings.ToLower(p.Key)] = p.Key
		protocolAlias[strings.ToLower(p.Name)] = p.Key
		for _, s := range p.Schemes {
			protocolScheme[strings.ToLower(s)] = p.Key
			protocolAlias[strings.ToLower(s)] = p.Key
		}
		for _, a := range p.Aliases {
			protocolAlias[strings.ToLower(a)] = p.Key
		}
	}
}

// Protocols returns the supported protocol registry.
func Protocols() []ProtocolInfo {
	out := make([]ProtocolInfo, len(protocolList))
	copy(out, protocolList)
	return out
}

// ProtocolInfoOf looks up a normalized protocol key.
func ProtocolInfoOf(key string) (ProtocolInfo, bool) {
	p, ok := protocolByKey[NormalizeProtocol(key)]
	return p, ok
}

// NormalizeProtocol maps a canonical key, scheme, or human alias to its
// key. Unknown names are returned lowercased with whitespace collapsed.
func NormalizeProtocol(raw string) string {
	key := strings.Join(strings.Fields(strings.ToLower(strings.TrimSpace(raw))), " ")
	if key == "" {
		return ""
	}
	if p, ok := protocolAlias[key]; ok {
		return p
	}
	if p, ok := protocolAlias[strings.ReplaceAll(key, "_", " ")]; ok {
		return p
	}
	return key
}

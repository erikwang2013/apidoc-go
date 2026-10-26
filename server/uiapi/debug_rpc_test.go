//go:build apidoc_debug_rpc || apidoc_debug_all

package uiapi

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/erikwang2013/apidoc-go/model"
	"github.com/erikwang2013/apidoc-go/store"
)

func debugRPCHandler(t *testing.T, protocol, method, target string, params, returned []model.Param, cfg DebugConfig, extra map[string]string) (http.Handler, string) {
	t.Helper()
	st := store.New("Debug")
	st.SetAppMeta("default", "Default")
	opts := map[string]string{"target": target}
	for key, value := range extra {
		opts[key] = value
	}
	doc := model.Doc{
		App: "default", Version: "v1", Controller: "debug", Title: "Debug",
		Protocol: protocol, ProtocolOpts: opts, Params: params,
	}
	if len(returned) > 0 {
		doc.Responses = []model.Response{{Name: "ok", Fields: returned}}
	}
	st.Register(method, target, doc)
	return newHandlerWith(t, Options{Store: st, Debug: cfg}), store.ActionID("default", "v1", method, target)
}

func thriftReply(method, message string) []byte {
	out := []byte{0x80, 0x01, 0x00, 0x02}
	out = binary.BigEndian.AppendUint32(out, uint32(len(method)))
	out = append(out, method...)
	out = binary.BigEndian.AppendUint32(out, 0)
	out = append(out, 11, 0, 1)
	out = binary.BigEndian.AppendUint32(out, uint32(len(message)))
	out = append(out, message...)
	return append(out, 0)
}

func TestRelayThriftTCP(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	serverErr := make(chan error, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			serverErr <- err
			return
		}
		defer conn.Close()
		request := make([]byte, 26)
		if _, err := io.ReadFull(conn, request); err != nil {
			serverErr <- err
			return
		}
		if !bytes.Equal(request[16:19], []byte{11, 0, 1}) ||
			!bytes.Equal(request[23:25], []byte("hi")) || request[25] != 0 {
			serverErr <- fmt.Errorf("unexpected thrift request: %x", request)
			return
		}
		_, err = conn.Write(thriftReply("ping", "pong"))
		serverErr <- err
	}()

	h, id := debugRPCHandler(t, "thrift", "POST", "tcp://"+ln.Addr().String(),
		[]model.Param{{Name: "name", In: "body", Type: "string", Field: "1", Required: true}},
		[]model.Param{{Name: "message", Type: "string", Field: "1"}},
		DebugConfig{EnableServerSide: true, AllowPrivateNetworks: true, TimeoutMS: 2000},
		map[string]string{"method": "ping"})
	data := debugSendData(t, h, id, `{"payload":"{\"name\":\"hi\"}","encoding":"utf8"}`)
	if data["ok"] != true {
		t.Fatalf("thrift tcp: %+v", data)
	}
	if data["body"] != `{"message":"pong"}` {
		t.Fatalf("thrift body = %v", data["body"])
	}
	headers := data["headers"].(map[string]any)
	if headers["x-thrift-method"] != "ping" {
		t.Fatalf("headers = %+v", headers)
	}
	if err := <-serverErr; err != nil {
		t.Fatalf("thrift server: %v", err)
	}
}

func TestRelayThriftHTTP(t *testing.T) {
	contentType := ""
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		contentType = r.Header.Get("Content-Type")
		_, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/x-thrift")
		_, _ = w.Write(thriftReply("ping", "http-pong"))
	}))
	defer mock.Close()

	h, id := debugRPCHandler(t, "thrift", "POST", mock.URL+"/thrift", nil,
		[]model.Param{{Name: "message", Type: "string", Field: "1"}},
		DebugConfig{EnableServerSide: true, AllowPrivateNetworks: true, TimeoutMS: 2000},
		map[string]string{"method": "ping", "transport": "http"})
	data := debugSendData(t, h, id, `{"payload":"{}","encoding":"utf8"}`)
	if data["ok"] != true || data["body"] != `{"message":"http-pong"}` {
		t.Fatalf("thrift http: %+v", data)
	}
	if contentType != "application/x-thrift" {
		t.Fatalf("content type = %q", contentType)
	}
}

func dubboReply(requestID []byte, value []byte) []byte {
	payload := append([]byte{0x01}, value...)
	out := []byte{0xda, 0xbb, 0x02, 20}
	out = append(out, requestID...)
	out = binary.BigEndian.AppendUint32(out, uint32(len(payload)))
	return append(out, payload...)
}

func startDubboMock(t *testing.T) (string, chan error) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	serverErr := make(chan error, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			serverErr <- err
			return
		}
		defer conn.Close()
		head := make([]byte, 16)
		if _, err := io.ReadFull(conn, head); err != nil {
			serverErr <- err
			return
		}
		if binary.BigEndian.Uint16(head[0:]) != 0xdabb || head[2] != 0xc2 || head[3] != 0 {
			serverErr <- fmt.Errorf("unexpected dubbo header: %x", head)
			return
		}
		length := binary.BigEndian.Uint32(head[12:])
		body := make([]byte, length)
		if _, err := io.ReadFull(conn, body); err != nil {
			serverErr <- err
			return
		}
		if len(body) == 0 || body[0] != 'S' {
			serverErr <- fmt.Errorf("unexpected dubbo body: %x", body)
			return
		}
		response := dubboReply(head[4:12], []byte{'S', 0, 11, 'h', 'e', 'l', 'l', 'o', ' ', 'w', 'o', 'r', 'l', 'd'})
		_, err = conn.Write(response)
		serverErr <- err
	}()
	return ln.Addr().String(), serverErr
}

func TestRelayDubboUnary(t *testing.T) {
	target, serverErr := startDubboMock(t)
	h, id := debugRPCHandler(t, "dubbo", "POST", "tcp://"+target, nil, nil,
		DebugConfig{EnableServerSide: true, AllowPrivateNetworks: true, TimeoutMS: 2000},
		map[string]string{
			"service": "com.example.DemoService", "method": "sayHello",
			"version": "1.0.0", "param_types": "Ljava/lang/String;",
		})
	data := debugSendData(t, h, id, `{"payload":"[\"world\"]","encoding":"utf8"}`)
	if data["ok"] != true || data["body"] != `"hello world"` {
		t.Fatalf("dubbo: %+v", data)
	}
	headers := data["headers"].(map[string]any)
	if headers["x-dubbo-status"] != "20" || headers["x-dubbo-method"] != "sayHello" {
		t.Fatalf("dubbo headers: %+v", headers)
	}
	if err := <-serverErr; err != nil {
		t.Fatalf("dubbo server: %v", err)
	}
}

func TestRelayHessianKind(t *testing.T) {
	target, serverErr := startDubboMock(t)
	h, id := debugRPCHandler(t, "hessian", "POST", "tcp://"+target, nil, nil,
		DebugConfig{EnableServerSide: true, AllowPrivateNetworks: true, TimeoutMS: 2000},
		map[string]string{
			"service": "com.example.DemoService", "method": "sayHello",
			"param_types": "Ljava/lang/String;",
		})
	if data := debugSendData(t, h, id, `{"payload":"[\"world\"]","encoding":"utf8"}`); data["ok"] != true {
		t.Fatalf("hessian: %+v", data)
	}
	if err := <-serverErr; err != nil {
		t.Fatalf("hessian server: %v", err)
	}
}

func TestRelayDubboParamTypesRequiredAndSSRF(t *testing.T) {
	h, id := debugRPCHandler(t, "dubbo", "POST", "tcp://127.0.0.1:9", nil, nil,
		DebugConfig{EnableServerSide: true, AllowPrivateNetworks: true, TimeoutMS: 500},
		map[string]string{"service": "s", "method": "m"})
	data := debugSendData(t, h, id, `{"payload":"[\"x\"]","encoding":"utf8"}`)
	if data["ok"] != false || !strings.Contains(data["error"].(string), "param_types") {
		t.Fatalf("param_types: %+v", data)
	}

	blocked, blockedID := debugRPCHandler(t, "dubbo", "POST", "tcp://127.0.0.1:9", nil, nil,
		DebugConfig{EnableServerSide: true, TimeoutMS: 500},
		map[string]string{
			"service": "s", "method": "m", "param_types": "Ljava/lang/String;",
		})
	data = debugSendData(t, blocked, blockedID, `{"payload":"[\"x\"]","encoding":"utf8"}`)
	if data["ok"] != false || !strings.Contains(data["error"].(string), "private") {
		t.Fatalf("dubbo ssrf: %+v", data)
	}
}

const rpcISOFields = `[
    {"field":"2","type":"llvar","len":19,"kind":"n"},
    {"field":"3","type":"fixed","len":6,"kind":"n"},
    {"field":"4","type":"fixed","len":12,"kind":"n"},
    {"field":"39","type":"fixed","len":2,"kind":"n"}
]`

func TestRelayISO8583Unary(t *testing.T) {
	goldenRequest := append([]byte("0200"), 0x70, 0, 0, 0, 0, 0, 0, 0)
	goldenRequest = append(goldenRequest, "164111111111111111000000000000000100"...)
	response := append([]byte("0210"), 0x40, 0, 0, 0, 0x02, 0, 0, 0)
	response = append(response, "164111111111111111"...)
	response = append(response, "00"...)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	serverErr := make(chan error, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			serverErr <- err
			return
		}
		defer conn.Close()
		head := make([]byte, 2)
		if _, err := io.ReadFull(conn, head); err != nil {
			serverErr <- err
			return
		}
		body := make([]byte, binary.BigEndian.Uint16(head))
		if _, err := io.ReadFull(conn, body); err != nil {
			serverErr <- err
			return
		}
		if !bytes.Equal(body, goldenRequest) {
			serverErr <- fmt.Errorf("unexpected iso8583 request: %x", body)
			return
		}
		frame := binary.BigEndian.AppendUint16(nil, uint16(len(response)))
		frame = append(frame, response...)
		_, err = conn.Write(frame)
		serverErr <- err
	}()

	h, id := debugRPCHandler(t, "iso8583", "POST", "tcp://"+ln.Addr().String(), nil, nil,
		DebugConfig{EnableServerSide: true, AllowPrivateNetworks: true, TimeoutMS: 2000},
		map[string]string{"mti": "0200", "fields": rpcISOFields})
	data := debugSendData(t, h, id, `{"payload":"{\"2\":\"4111111111111111\",\"3\":\"000000\",\"4\":\"000000000100\"}","encoding":"utf8"}`)
	if data["ok"] != true {
		t.Fatalf("iso8583: %+v", data)
	}
	var parsed struct {
		MTI    string            `json:"mti"`
		Fields map[string]string `json:"fields"`
	}
	if err := json.Unmarshal([]byte(data["body"].(string)), &parsed); err != nil {
		t.Fatal(err)
	}
	if parsed.MTI != "0210" || parsed.Fields["2"] != "4111111111111111" || parsed.Fields["39"] != "00" {
		t.Fatalf("iso8583 body: %+v", parsed)
	}
	if data["headers"].(map[string]any)["x-mti"] != "0210" {
		t.Fatalf("iso8583 headers: %+v", data["headers"])
	}
	if err := <-serverErr; err != nil {
		t.Fatalf("iso8583 server: %v", err)
	}
}

func TestRelayISO8583SSRFAndEBCDIC(t *testing.T) {
	blocked, blockedID := debugRPCHandler(t, "iso8583", "POST", "tcp://127.0.0.1:9", nil, nil,
		DebugConfig{EnableServerSide: true, TimeoutMS: 500},
		map[string]string{"mti": "0200", "fields": rpcISOFields})
	data := debugSendData(t, blocked, blockedID, `{"payload":"{}","encoding":"utf8"}`)
	if data["ok"] != false || !strings.Contains(data["error"].(string), "private") {
		t.Fatalf("iso8583 ssrf: %+v", data)
	}

	ebcdic, ebcdicID := debugRPCHandler(t, "iso8583", "POST", "tcp://127.0.0.1:9", nil, nil,
		DebugConfig{EnableServerSide: true, AllowPrivateNetworks: true},
		map[string]string{"mti": "0200", "fields": rpcISOFields, "encoding": "ebcdic"})
	data = debugSendData(t, ebcdic, ebcdicID, `{"payload":"{}","encoding":"utf8"}`)
	if data["ok"] != false || !strings.Contains(data["error"].(string), "ebcdic") {
		t.Fatalf("iso8583 ebcdic: %+v", data)
	}
}

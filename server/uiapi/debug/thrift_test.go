//go:build apidoc_debug_rpc || apidoc_debug_all

package debug

import (
	"bytes"
	"reflect"
	"strings"
	"testing"
)

func TestThriftMessageBeginVector(t *testing.T) {
	out := []byte{}
	thriftWriteMessageBegin(&out, "ping", 1, 0)
	want := []byte{0x80, 0x01, 0x00, 0x01, 0, 0, 0, 4, 'p', 'i', 'n', 'g', 0, 0, 0, 0}
	if !bytes.Equal(out, want) {
		t.Fatalf("message begin = %x, want %x", out, want)
	}
}

func TestThriftRequestAndResponse(t *testing.T) {
	body := &Payload{Encoding: "utf8", Data: `{"name":"hi"}`}
	req := &Request{
		Body: body,
		Endpoint: &Endpoint{
			Params: []Param{{Name: "name", Type: "string", Field: "1", Required: true}},
		},
	}
	frame, err := buildThriftRequest(req, "ping")
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{
		0x80, 0x01, 0x00, 0x01, 0, 0, 0, 4, 'p', 'i', 'n', 'g', 0, 0, 0, 0,
		11, 0, 1, 0, 0, 0, 2, 'h', 'i', 0,
	}
	if !bytes.Equal(frame, want) {
		t.Fatalf("request = %x, want %x", frame, want)
	}

	reply := []byte{
		0x80, 0x01, 0x00, 0x02, 0, 0, 0, 4, 'p', 'i', 'n', 'g', 0, 0, 0, 0,
		11, 0, 1, 0, 0, 0, 4, 'p', 'o', 'n', 'g', 0,
	}
	value, complete, err := parseThriftResponse(reply, &Endpoint{
		Returned: []Param{{Name: "message", Type: "string", Field: "1"}},
	})
	if err != nil || !complete {
		t.Fatalf("parse reply: complete=%v err=%v", complete, err)
	}
	if !reflect.DeepEqual(value, map[string]any{"message": "pong"}) {
		t.Fatalf("reply value = %#v", value)
	}

	partial := reply[:20]
	_, complete, err = parseThriftResponse(partial, nil)
	if err != nil || complete {
		t.Fatalf("partial reply: complete=%v err=%v", complete, err)
	}
}

func TestThriftExceptionIsReported(t *testing.T) {
	reply := []byte{
		0x80, 0x01, 0x00, 0x03, 0, 0, 0, 4, 'p', 'i', 'n', 'g', 0, 0, 0, 0,
		11, 0, 1, 0, 0, 0, 4, 'b', 'o', 'o', 'm', 0,
	}
	_, _, err := parseThriftResponse(reply, nil)
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("exception = %v", err)
	}
}

func TestThriftRequestByteCap(t *testing.T) {
	req := &Request{
		Kind:   "thrift",
		Target: "tcp://127.0.0.1:9",
		Body:   &Payload{Encoding: "utf8", Data: `{"name":"0123456789012345678901234567890123456789"}`},
		Opts:   map[string]string{"method": "ping"},
		Endpoint: &Endpoint{Params: []Param{{
			Name: "name", Type: "string", Field: "1",
		}}},
	}
	if _, err := executeThrift(Config{MaxBytes: 16}, req); err == nil ||
		!strings.Contains(err.Error(), "max_bytes") {
		t.Fatalf("request cap error = %v", err)
	}
}

func TestThriftMissingSchemaAndMethod(t *testing.T) {
	if _, err := executeThrift(Config{}, &Request{Opts: map[string]string{}}); err == nil ||
		!strings.Contains(err.Error(), "method") {
		t.Fatalf("missing method = %v", err)
	}
	if _, err := buildThriftRequest(&Request{Body: &Payload{}, Opts: map[string]string{}}, "ping"); err == nil ||
		!strings.Contains(err.Error(), "endpoint") {
		t.Fatalf("missing endpoint = %v", err)
	}
}

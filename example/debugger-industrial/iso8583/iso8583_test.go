package iso8583

import (
	"context"
	"encoding/json"
	"net"
	"testing"

	"github.com/erikwang2013/apidoc-go"
	iso "github.com/moov-io/iso8583"
	"github.com/moov-io/iso8583/specs"
)

// serveISO answers one length-prefixed request with a packed 0110 response.
func serveISO(t *testing.T, spec *iso.MessageSpec) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		req, err := readFrame(c, "2byte")
		if err != nil {
			return
		}
		m := iso.NewMessage(spec)
		if err := m.Unpack(req); err != nil {
			return
		}
		if mti, _ := m.GetMTI(); mti != "0100" {
			return
		}
		if pan, _ := m.GetString(2); pan != "4242424242424242" {
			return
		}
		resp := iso.NewMessage(spec)
		resp.MTI("0110")
		_ = resp.Field(2, "4242424242424242")
		_ = resp.Field(3, "000000")
		_ = resp.Field(39, "00")
		packed, err := resp.Pack()
		if err != nil {
			return
		}
		_ = writeFrame(c, packed, "2byte")
	}()
	return ln.Addr().String()
}

func body() []byte {
	return []byte(`{"mti":"0100","fields":{"2":"4242424242424242","3":"000000","4":"000000000100"}}`)
}

func TestSendAndUnpack(t *testing.T) {
	addr := serveISO(t, specs.Spec87ASCII)
	resp, err := New().Exec(context.Background(), apidoc.DebugRequest{
		Method: "SEND",
		URL:    "iso8583://" + addr,
		Body:   body(),
		Options: map[string]string{
			"spec": "87ascii", "length_prefix": "2byte", "timeout_ms": "2000",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	var out struct {
		MTI    string            `json:"mti"`
		Fields map[string]string `json:"fields"`
	}
	if err := json.Unmarshal(resp.Body, &out); err != nil {
		t.Fatal(err)
	}
	if out.MTI != "0110" || out.Fields["39"] != "00" || out.Fields["2"] != "4242424242424242" {
		t.Fatalf("response = %+v", out)
	}
}

func TestInlineJSONSpec(t *testing.T) {
	raw, err := specs.ExportJSON(specs.Spec87ASCII)
	if err != nil {
		t.Fatal(err)
	}
	addr := serveISO(t, specs.Spec87ASCII)
	resp, err := New().Exec(context.Background(), apidoc.DebugRequest{
		Method: "SEND",
		URL:    "iso8583://" + addr,
		Body:   body(),
		Options: map[string]string{
			"spec": string(raw), "length_prefix": "2byte", "timeout_ms": "2000",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Status != "OK" {
		t.Fatalf("status = %q", resp.Status)
	}
}

func TestBadSpec(t *testing.T) {
	if _, err := loadSpec("{not json"); err == nil {
		t.Fatal("want error for invalid inline spec")
	}
}

func TestFourASCIILength(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		if _, err := readFrame(c, "4ascii"); err != nil {
			return
		}
		resp := iso.NewMessage(specs.Spec87ASCII)
		resp.MTI("0110")
		_ = resp.Field(39, "00")
		packed, _ := resp.Pack()
		_ = writeFrame(c, packed, "4ascii")
	}()
	resp, err := New().Exec(context.Background(), apidoc.DebugRequest{
		Method: "SEND", URL: "iso8583://" + ln.Addr().String(), Body: body(),
		Options: map[string]string{"length_prefix": "4ascii", "timeout_ms": "2000"},
	})
	if err != nil {
		t.Fatal(err)
	}
	var out struct {
		MTI string `json:"mti"`
	}
	if err := json.Unmarshal(resp.Body, &out); err != nil || out.MTI != "0110" {
		t.Fatalf("response = %s err=%v", resp.Body, err)
	}
}

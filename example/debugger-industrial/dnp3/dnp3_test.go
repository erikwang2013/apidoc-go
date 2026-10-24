package dnp3

import (
	"context"
	"encoding/json"
	"net"
	"testing"

	"github.com/erikwang2013/apidoc-go"
)

var class0Response = []byte{
	0xC0, 0x81, 0x00, 0x00, // control, RESPONSE, IIN
	0x01, 0x02, 0x00, 0x00, 0x00, 0x81, // G1V2 start 0 stop 0: state + ONLINE
	0x1E, 0x02, 0x00, 0x00, 0x00, 0xD2, 0x04, 0x01, // G30V2 start 0 stop 0: 1234 + ONLINE
}

// serveOutstation answers one master connection with class0Response when it
// decodes a valid class 0 poll, and records the request.
func serveOutstation(t *testing.T, respond bool) (string, <-chan frameOut) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	requests := make(chan frameOut, 1)
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		f, err := readFrame(c)
		if err != nil {
			return
		}
		requests <- f
		if !respond {
			return
		}
		app := f.Application
		if app == nil || app.Function != 0x01 || app.RawHex != "c0013c0206" {
			return
		}
		_, _ = c.Write(frame(0x44, f.Source, f.Destination, append([]byte{0xC0}, class0Response...)))
	}()
	return ln.Addr().String(), requests
}

type resultJSON struct {
	RequestHex string `json:"request_hex"`
	Responses  []struct {
		Control     byte   `json:"control"`
		Destination uint16 `json:"destination"`
		Source      uint16 `json:"source"`
		Application struct {
			FunctionName string `json:"function_name"`
			IIN          uint16 `json:"iin"`
			Objects      []struct {
				Group     uint8  `json:"group"`
				Variation uint8  `json:"variation"`
				Count     uint32 `json:"count"`
				Points    []struct {
					Index uint32 `json:"index"`
					Value any    `json:"value"`
					Flags *uint8 `json:"flags"`
					Raw   string `json:"raw"`
				} `json:"points"`
			} `json:"objects"`
		} `json:"application"`
	} `json:"responses"`
}

func TestReadClass0(t *testing.T) {
	addr, requests := serveOutstation(t, true)
	resp, err := New().Exec(context.Background(), apidoc.DebugRequest{
		Method: "READ_CLASS0", URL: "dnp3://" + addr,
		Options: map[string]string{"outstation_addr": "10", "master_addr": "3", "timeout_ms": "3000"},
	})
	if err != nil {
		t.Fatal(err)
	}
	req := <-requests
	if req.Control != 0xC4 || req.Destination != 10 || req.Source != 3 {
		t.Fatalf("request = control %#x dest %d src %d", req.Control, req.Destination, req.Source)
	}
	var out resultJSON
	if err := json.Unmarshal(resp.Body, &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Responses) != 1 {
		t.Fatalf("responses = %+v", out.Responses)
	}
	r := out.Responses[0]
	if r.Control != 0x44 || r.Destination != 3 || r.Source != 10 {
		t.Fatalf("response = control %#x dest %d src %d", r.Control, r.Destination, r.Source)
	}
	if r.Application.FunctionName != "RESPONSE" || r.Application.IIN != 0 {
		t.Fatalf("application = %+v", r.Application)
	}
	if len(r.Application.Objects) != 2 {
		t.Fatalf("objects = %+v", r.Application.Objects)
	}
	binObj, analog := r.Application.Objects[0], r.Application.Objects[1]
	if binObj.Group != 1 || binObj.Variation != 2 || binObj.Count != 1 ||
		binObj.Points[0].Value != true || *binObj.Points[0].Flags != 0x01 {
		t.Fatalf("binary object = %+v", binObj)
	}
	if analog.Group != 30 || analog.Variation != 2 || analog.Count != 1 ||
		analog.Points[0].Value != float64(1234) || *analog.Points[0].Flags != 0x01 {
		t.Fatalf("analog object = %+v", analog)
	}
}

func TestRaw(t *testing.T) {
	addr, requests := serveOutstation(t, true)
	resp, err := New().Exec(context.Background(), apidoc.DebugRequest{
		Method: "RAW", URL: "dnp3://" + addr,
		Body:    []byte{0xC0, 0x01, 0x3C, 0x02, 0x06},
		Options: map[string]string{"timeout_ms": "3000"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if req := <-requests; req.Application == nil || req.Application.Function != 0x01 {
		t.Fatalf("request = %+v", req)
	}
	var out resultJSON
	if err := json.Unmarshal(resp.Body, &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Responses) != 1 || len(out.RequestHex) == 0 {
		t.Fatalf("result = %+v", out)
	}
}

func TestNoResponse(t *testing.T) {
	addr, _ := serveOutstation(t, false)
	_, err := New().Exec(context.Background(), apidoc.DebugRequest{
		Method: "READ_CLASS0", URL: "dnp3://" + addr,
		Options: map[string]string{"timeout_ms": "300"},
	})
	if err == nil {
		t.Fatal("want timeout error")
	}
}

func TestCRC(t *testing.T) {
	got := crc([]byte("123456789"))
	if got[0] != 0x82 || got[1] != 0xEA {
		t.Fatalf("crc = %x, want 82ea", got)
	}
}

func TestBadURL(t *testing.T) {
	for _, req := range []apidoc.DebugRequest{
		{Method: "READ_CLASS0", URL: "tcp://127.0.0.1:1"},
		{Method: "RAW", URL: "dnp3://127.0.0.1:1"},
	} {
		if _, err := New().Exec(context.Background(), req); err == nil {
			t.Fatalf("want error for %+v", req)
		}
	}
}

package bacnet

import (
	"context"
	"encoding/json"
	"net"
	"testing"

	"github.com/erikwang2013/apidoc-go"
	bacnet "github.com/ulbios/bacnet"
	"github.com/ulbios/bacnet/services"
)

// serveBACnet answers Who-Is, ReadProperty and WriteProperty on one UDP
// socket and reports decoded requests on got.
func serveBACnet(t *testing.T) (string, chan any) {
	t.Helper()
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pc.Close() })
	got := make(chan any, 8)
	go func() {
		buf := make([]byte, 1500)
		for {
			n, from, err := pc.ReadFrom(buf)
			if err != nil {
				return
			}
			msg, err := bacnet.Parse(buf[:n])
			if err != nil {
				got <- err
				continue
			}
			var reply []byte
			switch m := msg.(type) {
			case *services.UnconfirmedWhoIs:
				got <- "WHO_IS"
				reply, _ = bacnet.NewIAm(0, 999)
			case *services.ConfirmedReadProperty:
				d, err := m.Decode()
				if err != nil {
					got <- err
					continue
				}
				got <- d
				reply, _ = bacnet.NewCACK(services.ServiceConfirmedReadProperty,
					d.ObjectType, d.InstanceId, d.PropertyId, 21.5)
			case *services.ConfirmedWriteProperty:
				d, err := m.Decode()
				if err != nil {
					got <- err
					continue
				}
				got <- d
				reply, _ = bacnet.NewSACK(services.ServiceConfirmedWriteProperty)
			}
			if reply != nil {
				_, _ = pc.WriteTo(reply, from)
			}
		}
	}()
	return pc.LocalAddr().String(), got
}

type resJSON struct {
	RequestHex string           `json:"request_hex"`
	Responses  []map[string]any `json:"responses"`
}

func run(t *testing.T, req apidoc.DebugRequest) resJSON {
	t.Helper()
	resp, err := New().Exec(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	var out resJSON
	if err := json.Unmarshal(resp.Body, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestWhoIs(t *testing.T) {
	addr, got := serveBACnet(t)
	out := run(t, apidoc.DebugRequest{
		Method: "WHO_IS", URL: "bacnet://" + addr,
		Options: map[string]string{"timeout_ms": "3000"},
	})
	if msg := <-got; msg != "WHO_IS" {
		t.Fatalf("request = %+v", msg)
	}
	if len(out.Responses) != 1 || out.Responses[0]["type"] != "I_AM" {
		t.Fatalf("responses = %+v", out.Responses)
	}
	// The library's IAm encoder hard-codes device 321.
	if out.Responses[0]["device_id"] != float64(321) || out.Responses[0]["vendor_id"] != float64(999) ||
		out.Responses[0]["max_apdu_length"] != float64(1024) {
		t.Fatalf("I_AM = %+v", out.Responses[0])
	}
}

func TestReadProperty(t *testing.T) {
	addr, got := serveBACnet(t)
	out := run(t, apidoc.DebugRequest{
		Method: "READ_PROPERTY", URL: "bacnet://" + addr,
		Params: []apidoc.DebugValue{
			{Name: "object_type", Value: "0"}, {Name: "instance", Value: "5"},
			{Name: "property_id", Value: "85"},
		},
		Options: map[string]string{"timeout_ms": "3000"},
	})
	d, ok := (<-got).(services.ConfirmedReadPropertyDec)
	if !ok || d.ObjectType != 0 || d.InstanceId != 5 || d.PropertyId != 85 {
		t.Fatalf("request = %+v", d)
	}
	if len(out.Responses) != 1 || out.Responses[0]["type"] != "COMPLEX_ACK" ||
		out.Responses[0]["present_value"] != 21.5 || out.Responses[0]["instance"] != float64(5) {
		t.Fatalf("responses = %+v", out.Responses)
	}
}

func TestWriteProperty(t *testing.T) {
	addr, got := serveBACnet(t)
	out := run(t, apidoc.DebugRequest{
		Method: "WRITE_PROPERTY", URL: "bacnet://" + addr,
		Params: []apidoc.DebugValue{
			{Name: "object_type", Value: "0"}, {Name: "instance", Value: "5"},
			{Name: "property_id", Value: "85"}, {Name: "value", Value: "42.5"},
		},
		Options: map[string]string{"timeout_ms": "3000"},
	})
	d, ok := (<-got).(services.ConfirmedWritePropertyDec)
	if !ok || d.ObjectType != 0 || d.InstanceId != 5 || d.PropertyId != 85 || d.Value != 42.5 {
		t.Fatalf("request = %+v", d)
	}
	if len(out.Responses) != 1 || out.Responses[0]["type"] != "SIMPLE_ACK" ||
		out.Responses[0]["service"] != float64(services.ServiceConfirmedWriteProperty) {
		t.Fatalf("responses = %+v", out.Responses)
	}
}

func TestNoResponse(t *testing.T) {
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer pc.Close()
	_, err = New().Exec(context.Background(), apidoc.DebugRequest{
		Method: "WHO_IS", URL: "bacnet://" + pc.LocalAddr().String(),
		Options: map[string]string{"timeout_ms": "300"},
	})
	if err == nil {
		t.Fatal("want timeout error")
	}
}

func TestBadRequest(t *testing.T) {
	for _, req := range []apidoc.DebugRequest{
		{Method: "WHO_IS", URL: "udp://127.0.0.1:1"},
		{Method: "READ_PROPERTY", URL: "bacnet://127.0.0.1:1",
			Params: []apidoc.DebugValue{{Name: "object_type", Value: "9999"}}},
		{Method: "EXPLODE", URL: "bacnet://127.0.0.1:1"},
	} {
		if _, err := New().Exec(context.Background(), req); err == nil {
			t.Fatalf("want error for %+v", req)
		}
	}
}

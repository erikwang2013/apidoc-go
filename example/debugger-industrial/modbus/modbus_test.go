package modbus

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"io"
	"net"
	"testing"

	"github.com/erikwang2013/apidoc-go"
)

// serveMBAP answers one Modbus TCP connection with a fixed register/coil map.
func serveMBAP(t *testing.T) (addr string, writes chan []byte) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	writes = make(chan []byte, 8)
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		for {
			hdr := make([]byte, 7)
			if _, err := io.ReadFull(c, hdr); err != nil {
				return
			}
			length := int(binary.BigEndian.Uint16(hdr[4:6]))
			pdu := make([]byte, length-1)
			if _, err := io.ReadFull(c, pdu); err != nil {
				return
			}
			writes <- pdu
			resp := mbapResponse(hdr, pdu)
			if _, err := c.Write(resp); err != nil {
				return
			}
		}
	}()
	return ln.Addr().String(), writes
}

func mbapResponse(hdr, pdu []byte) []byte {
	fc := pdu[0]
	var data []byte
	switch fc {
	case 3, 4: // read holding / input registers
		qty := int(binary.BigEndian.Uint16(pdu[3:5]))
		data = make([]byte, 1+qty*2)
		data[0] = byte(qty * 2)
		for i := 0; i < qty; i++ {
			binary.BigEndian.PutUint16(data[1+i*2:], uint16(10*(i+1)))
		}
	case 1, 2: // read coils / discrete inputs: 1,0,1,1,...
		qty := int(binary.BigEndian.Uint16(pdu[3:5]))
		n := (qty + 7) / 8
		data = make([]byte, 1+n)
		data[0] = byte(n)
		bits := []bool{true, false, true, true}
		for i := 0; i < qty; i++ {
			if bits[i%len(bits)] {
				data[1+i/8] |= 1 << (i % 8)
			}
		}
	case 6: // write single register: echo
		data = pdu[1:5]
	case 16: // write multiple registers: address + quantity
		data = pdu[1:5]
	case 5: // write single coil: echo
		data = pdu[1:5]
	case 15: // write multiple coils: address + quantity
		data = pdu[1:5]
	default:
		data = []byte{0x01} // illegal function
	}
	out := make([]byte, 7, 7+1+len(data))
	copy(out, hdr)
	binary.BigEndian.PutUint16(out[4:6], uint16(2+len(data)))
	out = append(out, fc)
	out = append(out, data...)
	return out
}

func run(t *testing.T, addr, method string, params ...apidoc.DebugValue) *apidoc.DebugResponse {
	t.Helper()
	resp, err := New().Exec(context.Background(), apidoc.DebugRequest{
		Method:  method,
		URL:     "modbus://" + addr,
		Params:  params,
		Options: map[string]string{"unit_id": "1", "timeout_ms": "1000"},
	})
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func TestReadHoldingRegisters(t *testing.T) {
	addr, _ := serveMBAP(t)
	resp := run(t, addr, "READ_HOLDING_REGISTERS",
		apidoc.DebugValue{Name: "address", Value: "0"},
		apidoc.DebugValue{Name: "quantity", Value: "2"})
	var out struct {
		Values []uint16 `json:"values"`
	}
	if err := json.Unmarshal(resp.Body, &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Values) != 2 || out.Values[0] != 10 || out.Values[1] != 20 {
		t.Fatalf("values = %v", out.Values)
	}
}

func TestReadCoils(t *testing.T) {
	addr, _ := serveMBAP(t)
	resp := run(t, addr, "READ_COILS",
		apidoc.DebugValue{Name: "address", Value: "0"},
		apidoc.DebugValue{Name: "quantity", Value: "4"})
	var out struct {
		Values []bool `json:"values"`
	}
	if err := json.Unmarshal(resp.Body, &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Values) != 4 || !out.Values[0] || out.Values[1] || !out.Values[2] || !out.Values[3] {
		t.Fatalf("values = %v", out.Values)
	}
}

func TestWriteSingleRegister(t *testing.T) {
	addr, writes := serveMBAP(t)
	run(t, addr, "WRITE_SINGLE_REGISTER",
		apidoc.DebugValue{Name: "address", Value: "7"},
		apidoc.DebugValue{Name: "value", Value: "0x1234"})
	pdu := <-writes
	if pdu[0] != 6 || binary.BigEndian.Uint16(pdu[1:3]) != 7 || binary.BigEndian.Uint16(pdu[3:5]) != 0x1234 {
		t.Fatalf("pdu = %x", pdu)
	}
}

func TestUnknownAction(t *testing.T) {
	addr, _ := serveMBAP(t)
	_, err := New().Exec(context.Background(), apidoc.DebugRequest{
		Method: "EXPLODE", URL: "modbus://" + addr,
	})
	if err == nil {
		t.Fatal("want error for unknown action")
	}
}

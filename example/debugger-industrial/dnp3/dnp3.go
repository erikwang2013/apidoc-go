// Package dnp3 debugs DNP3 outstations with a hand-rolled link/transport/
// application stack: a class 0 integrity poll or a raw application PDU.
package dnp3

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/url"
	"time"

	"github.com/erikwang2013/apidoc-go"
	"github.com/erikwang2013/apidoc-go/example/debugger-industrial/internal/opts"
)

const (
	linkHeaderLen = 10 // start(2) length(1) control(1) dest(2) src(2) crc(2)
	maxUserData   = 16
)

// New returns the DNP3 debugger. The URL is dnp3://host:port; RAW sends the
// request body as an application PDU (application control byte included),
// READ_CLASS0 performs a class 0 integrity poll.
func New() apidoc.Debugger {
	return apidoc.DebuggerFunc("dnp3", apidoc.DebugSpec{
		Mode:         "server",
		Actions:      []string{"READ_CLASS0", "RAW"},
		Encodings:    []string{"hex", "json"},
		ResponseView: "json",
		ConnectFields: []apidoc.DebugField{
			{Name: "outstation_addr", Label: "Outstation link address", Type: "number", Default: "1"},
			{Name: "master_addr", Label: "Master link address", Type: "number", Default: "0"},
			{Name: "timeout_ms", Label: "Timeout (ms)", Type: "number", Default: "5000"},
		},
	}, exec)
}

func exec(ctx context.Context, req apidoc.DebugRequest) (*apidoc.DebugResponse, error) {
	u, err := url.Parse(req.URL)
	if err != nil || u.Scheme != "dnp3" || u.Hostname() == "" {
		return nil, errors.New("dnp3: url must be dnp3://host:port")
	}
	addr := u.Host
	if u.Port() == "" {
		addr = net.JoinHostPort(u.Hostname(), "20000")
	}
	dest, err := opts.Uint(req.Options, "outstation_addr", 1, 65534)
	if err != nil {
		return nil, err
	}
	src, err := opts.Uint(req.Options, "master_addr", 0, 65534)
	if err != nil {
		return nil, err
	}
	timeout, err := opts.Timeout(req.Options, "timeout_ms", 5*time.Second, 30*time.Second)
	if err != nil {
		return nil, err
	}

	var app []byte
	switch req.Method {
	case "READ_CLASS0":
		app = []byte{0xC0, 0x01, 0x3C, 0x02, 0x06} // FIR|FIN, READ, class 0 all objects
	case "RAW":
		if len(req.Body) == 0 {
			return nil, errors.New("dnp3: RAW requires an application PDU body")
		}
		if len(req.Body) > 249 {
			return nil, errors.New("dnp3: application PDU exceeds one link frame")
		}
		app = req.Body
	default:
		return nil, fmt.Errorf("dnp3: unknown action %q", req.Method)
	}
	request := frame(0xC4, uint16(dest), uint16(src), append([]byte{0xC0}, app...))

	d := net.Dialer{Timeout: timeout}
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	if _, err := conn.Write(request); err != nil {
		return nil, err
	}

	overall := time.Now().Add(timeout)
	var frames []frameOut
	for {
		if time.Now().After(overall) {
			return nil, errors.New("dnp3: no response before timeout")
		}
		_ = conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		f, err := readFrame(conn)
		if err != nil {
			if len(frames) > 0 { // outstation closed after its last response
				break
			}
			var ne net.Error
			if errors.As(err, &ne) && ne.Timeout() {
				continue
			}
			return nil, err
		}
		frames = append(frames, f)
		if err := ctx.Err(); err != nil {
			return nil, err
		}
	}
	body, _ := json.Marshal(map[string]any{
		"request_hex": hex.EncodeToString(request),
		"responses":   frames,
	})
	return &apidoc.DebugResponse{Status: "OK", Body: body, Encoding: "json"}, nil
}

func frame(control byte, dest, src uint16, user []byte) []byte {
	hdr := []byte{0x05, 0x64, byte(5 + len(user)), control,
		byte(dest), byte(dest >> 8), byte(src), byte(src >> 8)}
	out := append(hdr, crc(hdr[2:])...)
	for len(user) > 0 {
		n := min(maxUserData, len(user))
		out = append(out, user[:n]...)
		out = append(out, crc(user[:n])...)
		user = user[n:]
	}
	return out
}

// crc is CRC-16/DNP: reflected polynomial 0xA6BC, init 0, complemented.
func crc(data []byte) []byte {
	var c uint16
	for _, b := range data {
		c ^= uint16(b)
		for i := 0; i < 8; i++ {
			if c&1 != 0 {
				c = c>>1 ^ 0xA6BC
			} else {
				c >>= 1
			}
		}
	}
	c = ^c
	return []byte{byte(c), byte(c >> 8)}
}

type transportOut struct {
	FIR bool  `json:"fir"`
	FIN bool  `json:"fin"`
	SEQ uint8 `json:"seq"`
}

type frameOut struct {
	RawHex      string       `json:"raw_hex"`
	Control     byte         `json:"control"`
	Destination uint16       `json:"destination"`
	Source      uint16       `json:"source"`
	Transport   transportOut `json:"transport"`
	Application *application `json:"application,omitempty"`
}

func readFrame(r io.Reader) (frameOut, error) {
	var out frameOut
	hdr := make([]byte, linkHeaderLen)
	if _, err := io.ReadFull(r, hdr); err != nil {
		return out, err
	}
	if hdr[0] != 0x05 || hdr[1] != 0x64 {
		return out, fmt.Errorf("dnp3: bad link start %x", hdr[:2])
	}
	if binary.LittleEndian.Uint16(hdr[8:]) != binary.LittleEndian.Uint16(crc(hdr[2:8])) {
		return out, errors.New("dnp3: link header CRC mismatch")
	}
	length := int(hdr[2])
	if length < 5 {
		return out, fmt.Errorf("dnp3: bad link length %d", length)
	}
	userLen := length - 5
	user := make([]byte, 0, userLen)
	raw := append([]byte{}, hdr...)
	for len(user) < userLen {
		n := min(maxUserData, userLen-len(user))
		block := make([]byte, n+2)
		if _, err := io.ReadFull(r, block); err != nil {
			return out, err
		}
		if binary.LittleEndian.Uint16(block[n:]) != binary.LittleEndian.Uint16(crc(block[:n])) {
			return out, errors.New("dnp3: user data CRC mismatch")
		}
		user = append(user, block[:n]...)
		raw = append(raw, block...)
	}
	out.RawHex = hex.EncodeToString(raw)
	out.Control = hdr[3]
	out.Destination = binary.LittleEndian.Uint16(hdr[4:6])
	out.Source = binary.LittleEndian.Uint16(hdr[6:8])
	if len(user) == 0 {
		return out, nil
	}
	out.Transport = transportOut{FIR: user[0]&0x40 != 0, FIN: user[0]&0x80 != 0, SEQ: user[0] & 0x3F}
	if user[0]&0xC0 == 0xC0 {
		out.Application = parseApplication(user[1:])
	}
	return out, nil
}

type application struct {
	Control      byte     `json:"control"`
	Function     byte     `json:"function"`
	FunctionName string   `json:"function_name,omitempty"`
	IIN          uint16   `json:"iin"`
	IINBits      []string `json:"iin_bits,omitempty"`
	Objects      []object `json:"objects,omitempty"`
	RawHex       string   `json:"raw_hex,omitempty"`
}

func parseApplication(pdu []byte) *application {
	if len(pdu) < 2 {
		return &application{RawHex: hex.EncodeToString(pdu)}
	}
	app := &application{Control: pdu[0], Function: pdu[1], RawHex: hex.EncodeToString(pdu)}
	switch pdu[1] {
	case 0x81:
		app.FunctionName = "RESPONSE"
	case 0x82:
		app.FunctionName = "UNSOLICITED_RESPONSE"
	}
	if pdu[1] != 0x81 && pdu[1] != 0x82 {
		return app
	}
	if len(pdu) < 4 {
		return app
	}
	app.IIN = uint16(pdu[3])<<8 | uint16(pdu[2])
	app.IINBits = iinBits(pdu[2], pdu[3])
	app.Objects, app.RawHex = parseObjects(pdu[4:])
	return app
}

func iinBits(iin1, iin2 byte) []string {
	bits := []string{
		"BROADCAST", "CLASS1_EVENTS", "CLASS2_EVENTS", "CLASS3_EVENTS",
		"NEED_TIME", "LOCAL_CONTROL", "DEVICE_TROUBLE", "DEVICE_RESTART",
		"FUNC_NOT_SUPPORTED", "OBJECT_UNKNOWN", "PARAM_ERROR", "EVENT_BUFFER_OVERFLOW",
		"ALREADY_EXECUTING", "CONFIG_CORRUPT",
	}
	var out []string
	for i, name := range bits {
		var set bool
		if i < 8 {
			set = iin1&(1<<i) != 0
		} else {
			set = iin2&(1<<(i-8)) != 0
		}
		if set {
			out = append(out, name)
		}
	}
	return out
}

type object struct {
	Group     uint8   `json:"group"`
	Variation uint8   `json:"variation"`
	Qualifier uint8   `json:"qualifier"`
	Start     uint32  `json:"start,omitempty"`
	Count     uint32  `json:"count"`
	Points    []point `json:"points,omitempty"`
	RawHex    string  `json:"raw_hex,omitempty"`
}

type point struct {
	Index uint32 `json:"index"`
	Value any    `json:"value,omitempty"`
	Flags *uint8 `json:"flags,omitempty"`
	Raw   string `json:"raw"`
}

// pointSize is the wire size of one object occurrence; 0 means this adapter
// does not decode the group/variation.
func pointSize(group, variation uint8) int {
	switch group {
	case 1, 10: // binary input/output
		if variation == 2 {
			return 1
		}
	case 20: // counter
		switch variation {
		case 1, 5:
			return 5
		case 2, 6:
			return 3
		}
	case 30: // analog input
		switch variation {
		case 1, 5:
			return 5
		case 2:
			return 3
		case 3:
			return 4
		case 4:
			return 2
		case 6:
			return 9
		}
	case 40: // analog output
		switch variation {
		case 1, 3:
			return 5
		case 2, 4:
			return 9
		}
	}
	return 0
}

func parsePoint(group, variation uint8, index uint32, b []byte) point {
	p := point{Index: index, Raw: hex.EncodeToString(b)}
	flags := b[len(b)-1]
	switch group {
	case 1, 10:
		if variation == 2 {
			v := b[0]>>7 != 0
			f := b[0] & 0x7F
			p.Value, p.Flags = v, &f
		}
	case 20:
		switch variation {
		case 1:
			v := binary.LittleEndian.Uint32(b)
			p.Value, p.Flags = v, &flags
		case 2:
			v := binary.LittleEndian.Uint16(b)
			p.Value, p.Flags = v, &flags
		}
	case 30:
		switch variation {
		case 1:
			v := int32(binary.LittleEndian.Uint32(b))
			p.Value, p.Flags = v, &flags
		case 2:
			v := int16(binary.LittleEndian.Uint16(b))
			p.Value, p.Flags = v, &flags
		case 3:
			p.Value = int32(binary.LittleEndian.Uint32(b))
		case 4:
			p.Value = int16(binary.LittleEndian.Uint16(b))
		case 5:
			v := float32From(binary.LittleEndian.Uint32(b))
			p.Value, p.Flags = v, &flags
		case 6:
			v := float64From(binary.LittleEndian.Uint64(b))
			p.Value, p.Flags = v, &flags
		}
	case 40:
		switch variation {
		case 1, 3:
			p.Value = float32From(binary.LittleEndian.Uint32(b))
		case 2, 4:
			p.Value = float64From(binary.LittleEndian.Uint64(b))
		}
		if variation == 1 || variation == 2 {
			p.Flags = &flags
		}
	}
	return p
}

func parseObjects(data []byte) ([]object, string) {
	var objs []object
	for len(data) >= 3 {
		o := object{Group: data[0], Variation: data[1], Qualifier: data[2]}
		data = data[3:]
		size := pointSize(o.Group, o.Variation)
		prefix := o.Qualifier & 0x0F
		if prefix != 0 || size == 0 {
			o.RawHex = hex.EncodeToString(data)
			return append(objs, o), ""
		}
		var count uint32
		switch o.Qualifier >> 4 {
		case 0, 1: // start/stop
			width := 1 << (o.Qualifier >> 4)
			if len(data) < 2*width {
				o.RawHex = hex.EncodeToString(data)
				return append(objs, o), ""
			}
			o.Start = readUint(data[:width])
			stop := readUint(data[width : 2*width])
			data = data[2*width:]
			if stop < o.Start {
				o.RawHex = hex.EncodeToString(data)
				return append(objs, o), ""
			}
			count = stop - o.Start + 1
		case 6: // all objects: count inferred from the remainder
			count = uint32(len(data) / size)
		case 7: // 1-octet count
			if len(data) < 1 {
				return append(objs, o), ""
			}
			count = uint32(data[0])
			data = data[1:]
		default:
			o.RawHex = hex.EncodeToString(data)
			return append(objs, o), ""
		}
		if int(count)*size > len(data) {
			o.RawHex = hex.EncodeToString(data)
			return append(objs, o), ""
		}
		o.Count = count
		for i := 0; i < int(count); i++ {
			b := data[i*size : (i+1)*size]
			o.Points = append(o.Points, parsePoint(o.Group, o.Variation, o.Start+uint32(i), b))
		}
		data = data[int(count)*size:]
		objs = append(objs, o)
	}
	return objs, hex.EncodeToString(data)
}

func readUint(b []byte) uint32 {
	var v uint32
	for i := len(b) - 1; i >= 0; i-- {
		v = v<<8 | uint32(b[i])
	}
	return v
}

func float32From(v uint32) float32 { return math.Float32frombits(v) }

func float64From(v uint64) float64 { return math.Float64frombits(v) }

//go:build apidoc_debug_industrial || apidoc_debug_all

package debug

import (
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"time"
)

const (
	dnp3DefaultPort = "20000"
	dnp3MaxPoints   = 10000 // bounds packed-bit and multi-frame amplification
)

func init() {
	RegisterExecutor("dnp3", executeDNP3)
}

// dnp3DNP3 is a read-only subset: link layer, transport reassembly and
// Integrity/Class 0..3 polls. Events, selects/operates, time sync, file
// transfer and secure authentication are out of scope.
func executeDNP3(cfg Config, req *Request) (*Response, error) {
	started := time.Now()
	host, port, err := parseProtocolTarget(req.Target, "dnp3", dnp3DefaultPort, "dnp3", "tcp")
	if err != nil {
		return nil, err
	}
	linkAddress := dnp3OptU16(req, "link_address", 1)
	masterAddress := dnp3OptU16(req, "source_address", 0)
	if linkAddress == masterAddress {
		return nil, fmt.Errorf("dnp3 link_address and source_address must differ")
	}
	duration := protocolWindow(cfg, req)
	operation := req.Opts["operation"]
	if operation == "" {
		operation = "integrity"
	}
	appPayload, err := dnp3ReadRequest(operation, req)
	if err != nil {
		return nil, err
	}
	app := append([]byte{0xC0, 0x01}, appPayload...) // FIR+FIN, Function Code = Read
	userData := append([]byte{0xC0}, app...)         // transport FIR+FIN, SEQ=0
	request, err := dnp3EncodeLinkFrame(0xC4, linkAddress, masterAddress, userData)
	if err != nil {
		return nil, err
	}

	conn, err := dialPinned(cfg, "tcp", host, port, req.Timeout(cfg))
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	if err := conn.SetWriteDeadline(time.Now().Add(req.Timeout(cfg))); err != nil {
		return nil, fmt.Errorf("dnp3 deadline failed: %v", err)
	}
	if _, err := conn.Write(request); err != nil {
		return nil, fmt.Errorf("dnp3 write failed: %v", err)
	}
	deadline := time.Now().Add(duration)
	points, err := dnp3CollectPoints(cfg, conn, deadline, linkAddress, masterAddress)
	if err != nil {
		return nil, err
	}
	if len(points) == 0 {
		return nil, fmt.Errorf("dnp3 timed out after %dms without a data response", duration.Milliseconds())
	}
	encoded, err := json.Marshal(map[string]any{"points": points})
	if err != nil {
		return nil, fmt.Errorf("encode response: %v", err)
	}
	return &Response{
		Status:  0,
		MS:      time.Since(started).Milliseconds(),
		Headers: map[string]string{"x-peer": conn.RemoteAddr().String()},
		Body:    FromBytes(encoded),
	}, nil
}

func dnp3ReadRequest(operation string, req *Request) ([]byte, error) {
	switch operation {
	case "integrity", "class0":
		return []byte{0x3C, 0x01, 0x06}, nil
	case "class1":
		return []byte{0x3C, 0x02, 0x06}, nil
	case "class2":
		return []byte{0x3C, 0x03, 0x06}, nil
	case "class3":
		return []byte{0x3C, 0x04, 0x06}, nil
	case "read":
		group := dnp3OptU8(req, "group", 30)
		variation := dnp3OptU8(req, "variation", 1)
		start := dnp3OptU32(req, "start", 0)
		stop := dnp3OptU32(req, "stop", start)
		if start > stop {
			return nil, fmt.Errorf("dnp3 start must be <= stop")
		}
		if stop > 0xFFFF {
			return nil, fmt.Errorf("dnp3 stop must be <= 65535")
		}
		out := []byte{group, variation}
		if stop <= 0xFF {
			out = append(out, 0x00, byte(start), byte(stop))
		} else {
			out = append(out, 0x01, byte(start), byte(start>>8), byte(stop), byte(stop>>8))
		}
		return out, nil
	default:
		return nil, fmt.Errorf("unsupported dnp3 operation `%s`; use integrity, class0, class1, "+
			"class2, class3 or read", operation)
	}
}

func dnp3CollectPoints(cfg Config, conn net.Conn, deadline time.Time, linkAddress, masterAddress uint16) ([]any, error) {
	points := []any{}
	app := []byte{}
	totalBytes := 0
	frames := 0
	for frames < 1000 {
		frame, err := dnp3ReadLinkFrame(conn, deadline)
		if err != nil {
			return nil, err
		}
		if frame == nil {
			break
		}
		totalBytes += len(frame.data) + 10
		if totalBytes > cfg.Limit() {
			return nil, fmt.Errorf("dnp3 response exceeds max_bytes (%d)", cfg.Limit())
		}
		// Only user-data frames addressed to the master and sent by the outstation.
		if frame.ctrl&0xC0 != 0x40 || frame.dst != masterAddress || frame.src != linkAddress {
			continue
		}
		if len(frame.data) == 0 {
			continue
		}
		transport := frame.data[0]
		if transport&0x80 != 0 {
			app = app[:0]
		}
		app = append(app, frame.data[1:]...)
		if transport&0x40 == 0 {
			continue // fragment not complete
		}
		if len(app) < 2 {
			app = app[:0]
			continue
		}
		if app[1] != 0x81 {
			app = app[:0] // Confirm/Unsolicited and other non-read responses
			continue
		}
		if len(app) < 4 {
			return nil, fmt.Errorf("truncated dnp3 read response (missing IIN)")
		}
		// [app control][function 0x81][IIN1][IIN2][object headers...]
		parsed, err := dnp3ParseObjects(app[4:])
		if err != nil {
			return nil, err
		}
		if len(parsed) > dnp3MaxPoints-len(points) {
			return nil, fmt.Errorf("dnp3 response exceeds %d points", dnp3MaxPoints)
		}
		points = append(points, parsed...)
		app = app[:0]
		frames++
	}
	return points, nil
}

func dnp3OptU8(req *Request, key string, def uint8) uint8 {
	if value, err := strconv.ParseUint(req.Opts[key], 10, 8); err == nil {
		return uint8(value)
	}
	return def
}

func dnp3OptU16(req *Request, key string, def uint16) uint16 {
	if value, err := strconv.ParseUint(req.Opts[key], 10, 16); err == nil {
		return uint16(value)
	}
	return def
}

func dnp3OptU32(req *Request, key string, def uint32) uint32 {
	if value, err := strconv.ParseUint(req.Opts[key], 10, 32); err == nil {
		return uint32(value)
	}
	return def
}

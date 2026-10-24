// Package bacnet debugs BACnet/IP devices with ulbios/bacnet: Who-Is,
// ReadProperty and WriteProperty (float32 values).
package bacnet

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"time"

	"github.com/erikwang2013/apidoc-go"
	"github.com/erikwang2013/apidoc-go/example/debugger-industrial/internal/opts"
	bacnet "github.com/ulbios/bacnet"
	"github.com/ulbios/bacnet/services"
)

// New returns the BACnet/IP debugger. The URL is bacnet://host:port (port
// defaults to 47808); object identifiers come from the request parameters.
func New() apidoc.Debugger {
	return apidoc.DebuggerFunc("bacnet", apidoc.DebugSpec{
		Mode:         "server",
		Actions:      []string{"WHO_IS", "READ_PROPERTY", "WRITE_PROPERTY"},
		Encodings:    []string{"json"},
		ResponseView: "json",
		ConnectFields: []apidoc.DebugField{
			{Name: "timeout_ms", Label: "Timeout (ms)", Type: "number", Default: "5000"},
		},
	}, exec)
}

func exec(ctx context.Context, req apidoc.DebugRequest) (*apidoc.DebugResponse, error) {
	u, err := url.Parse(req.URL)
	if err != nil || u.Scheme != "bacnet" || u.Hostname() == "" {
		return nil, errors.New("bacnet: url must be bacnet://host:port")
	}
	addr := u.Host
	if u.Port() == "" {
		addr = net.JoinHostPort(u.Hostname(), "47808")
	}
	timeout, err := opts.Timeout(req.Options, "timeout_ms", 5*time.Second, 30*time.Second)
	if err != nil {
		return nil, err
	}

	var request []byte
	switch req.Method {
	case "WHO_IS":
		request, err = bacnet.NewWhois()
	case "READ_PROPERTY":
		objectType, instance, property, perr := objectParams(req)
		if perr != nil {
			return nil, perr
		}
		request, err = bacnet.NewReadProperty(objectType, instance, property)
	case "WRITE_PROPERTY":
		objectType, instance, property, perr := objectParams(req)
		if perr != nil {
			return nil, perr
		}
		value, perr := strconv.ParseFloat(param(req, "value"), 32)
		if perr != nil {
			return nil, fmt.Errorf("bacnet: value: %w", perr)
		}
		request, err = bacnet.NewWriteProperty(objectType, instance, property, float32(value))
	default:
		return nil, fmt.Errorf("bacnet: unknown action %q", req.Method)
	}
	if err != nil {
		return nil, err
	}

	d := net.Dialer{Timeout: timeout}
	conn, err := d.DialContext(ctx, "udp", addr)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	if _, err := conn.Write(request); err != nil {
		return nil, err
	}

	overall := time.Now().Add(timeout)
	var responses []map[string]any
	buf := make([]byte, 1500)
	for {
		if time.Now().After(overall) {
			break
		}
		_ = conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		n, err := conn.Read(buf)
		if err != nil {
			var ne net.Error
			if errors.As(err, &ne) && ne.Timeout() {
				if len(responses) > 0 {
					break
				}
				continue
			}
			if len(responses) > 0 {
				break
			}
			return nil, err
		}
		responses = append(responses, decodeResponse(buf[:n]))
	}
	if len(responses) == 0 {
		return nil, errors.New("bacnet: no response before timeout")
	}
	body, _ := json.Marshal(map[string]any{
		"request_hex": hex.EncodeToString(request),
		"responses":   responses,
	})
	return &apidoc.DebugResponse{Status: "OK", Body: body, Encoding: "json"}, nil
}

func objectParams(req apidoc.DebugRequest) (objectType uint16, instance uint32, property uint8, err error) {
	ot, err := uintParam(req, "object_type", 0, 1023)
	if err != nil {
		return
	}
	inst, err := uintParam(req, "instance", 0, 4194303)
	if err != nil {
		return
	}
	prop, err := uintParam(req, "property_id", 85, 255) // 85 = present-value
	return uint16(ot), uint32(inst), uint8(prop), err
}

func param(req apidoc.DebugRequest, name string) string {
	for _, p := range req.Params {
		if p.Name == name && p.Value != "" {
			return p.Value
		}
	}
	return req.Options[name]
}

func uintParam(req apidoc.DebugRequest, name string, def, max uint64) (uint64, error) {
	s := param(req, name)
	if s == "" {
		return def, nil
	}
	v, err := strconv.ParseUint(s, 0, 64)
	if err != nil {
		return 0, fmt.Errorf("bacnet: %s: %w", name, err)
	}
	if v > max {
		return 0, fmt.Errorf("bacnet: %s %d out of range", name, v)
	}
	return v, nil
}

func decodeResponse(raw []byte) map[string]any {
	out := map[string]any{"raw_hex": hex.EncodeToString(raw), "type": "UNPARSEABLE"}
	msg, err := bacnet.Parse(raw)
	if err != nil {
		out["error"] = err.Error()
		return out
	}
	switch m := msg.(type) {
	case *services.UnconfirmedIAm:
		d, err := m.Decode()
		if err != nil {
			out["error"] = err.Error()
			return out
		}
		out["type"] = "I_AM"
		out["device_id"] = d.DeviceId
		out["max_apdu_length"] = d.MaxAPDULength
		out["segmentation_supported"] = d.SegmentationSupported
		out["vendor_id"] = d.VendorId
	case *services.ComplexACK:
		d, err := m.Decode()
		if err != nil {
			out["error"] = err.Error()
			return out
		}
		out["type"] = "COMPLEX_ACK"
		out["object_type"] = d.ObjectType
		out["instance"] = d.InstanceId
		out["property_id"] = d.PropertyId
		out["present_value"] = d.PresentValue
	case *services.SimpleACK:
		out["type"] = "SIMPLE_ACK"
		out["service"] = m.APDU.Service
	case *services.Error:
		d, err := m.Decode()
		if err != nil {
			out["error"] = err.Error()
			return out
		}
		out["type"] = "ERROR"
		out["service"] = m.APDU.Service
		out["error_class"] = d.ErrorClass
		out["error_code"] = d.ErrorCode
	case *services.UnconfirmedWhoIs:
		out["type"] = "WHO_IS"
	}
	return out
}

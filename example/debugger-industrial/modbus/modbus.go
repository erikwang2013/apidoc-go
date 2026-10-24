// Package modbus debugs Modbus TCP devices with the grid-x client.
package modbus

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/erikwang2013/apidoc-go"
	"github.com/erikwang2013/apidoc-go/example/debugger-industrial/internal/opts"
	"github.com/grid-x/modbus"
)

// New returns the Modbus TCP debugger.
func New() apidoc.Debugger {
	return apidoc.DebuggerFunc("modbus", apidoc.DebugSpec{
		Mode:         "server",
		Actions:      Actions,
		Encodings:    []string{"json", "text"},
		ResponseView: "json",
		ConnectFields: []apidoc.DebugField{
			{Name: "unit_id", Label: "Unit / slave ID", Type: "number", Default: "1"},
			{Name: "timeout_ms", Label: "Timeout (ms)", Type: "number", Default: "3000"},
		},
	}, exec)
}

// Actions supported by the adapter.
var Actions = []string{
	"READ_HOLDING_REGISTERS", "READ_INPUT_REGISTERS", "READ_COILS", "READ_DISCRETE_INPUTS",
	"WRITE_SINGLE_REGISTER", "WRITE_MULTIPLE_REGISTERS", "WRITE_SINGLE_COIL", "WRITE_MULTIPLE_COILS",
}

func exec(ctx context.Context, req apidoc.DebugRequest) (*apidoc.DebugResponse, error) {
	u, err := url.Parse(req.URL)
	if err != nil || u.Scheme != "modbus" || u.Host == "" {
		return nil, errors.New("modbus: url must be modbus://host:port")
	}
	unit, err := opts.Uint(req.Options, "unit_id", 1, 255)
	if err != nil {
		return nil, err
	}
	timeout, err := opts.Timeout(req.Options, "timeout_ms", 3*time.Second, 30*time.Second)
	if err != nil {
		return nil, err
	}
	address, err := paramUint(req, "address", 0, 65535)
	if err != nil {
		return nil, err
	}
	quantity, err := paramUint(req, "quantity", 1, 2000)
	if err != nil {
		return nil, err
	}

	handler := modbus.NewTCPClientHandler(u.Host)
	handler.SetSlave(byte(unit))
	handler.Timeout = timeout
	handler.IdleTimeout = -1
	defer handler.Close()
	client := modbus.NewClient(handler)

	var values any
	var raw []byte
	switch req.Method {
	case "READ_HOLDING_REGISTERS", "READ_INPUT_REGISTERS":
		if quantity > 125 {
			return nil, errors.New("modbus: quantity max 125 for registers")
		}
		if req.Method == "READ_HOLDING_REGISTERS" {
			raw, err = client.ReadHoldingRegisters(ctx, uint16(address), uint16(quantity))
		} else {
			raw, err = client.ReadInputRegisters(ctx, uint16(address), uint16(quantity))
		}
		if err == nil {
			regs := make([]uint16, len(raw)/2)
			for i := range regs {
				regs[i] = binary.BigEndian.Uint16(raw[i*2:])
			}
			values = regs
		}
	case "READ_COILS", "READ_DISCRETE_INPUTS":
		if quantity > 2000 {
			return nil, errors.New("modbus: quantity max 2000 for coils")
		}
		if req.Method == "READ_COILS" {
			raw, err = client.ReadCoils(ctx, uint16(address), uint16(quantity))
		} else {
			raw, err = client.ReadDiscreteInputs(ctx, uint16(address), uint16(quantity))
		}
		if err == nil {
			bits := make([]bool, quantity)
			for i := range bits {
				bits[i] = raw[i/8]&(1<<(i%8)) != 0
			}
			values = bits
		}
	case "WRITE_SINGLE_REGISTER":
		v, verr := paramUint(req, "value", 0, 65535)
		if verr != nil {
			return nil, verr
		}
		raw, err = client.WriteSingleRegister(ctx, uint16(address), uint16(v))
		values = v
	case "WRITE_MULTIPLE_REGISTERS":
		regs, verr := paramRegisters(req)
		if verr != nil {
			return nil, verr
		}
		if len(regs) == 0 || len(regs) > 123 {
			return nil, errors.New("modbus: values must contain 1..123 registers")
		}
		raw, err = client.WriteMultipleRegisters(ctx, uint16(address), uint16(len(regs)), uint16sToBytes(regs))
		quantity = uint64(len(regs))
		values = regs
	case "WRITE_SINGLE_COIL":
		v, verr := paramUint(req, "value", 0, 1)
		if verr != nil {
			return nil, verr
		}
		raw, err = client.WriteSingleCoil(ctx, uint16(address), uint16(v))
		values = v == 1
	case "WRITE_MULTIPLE_COILS":
		bits, verr := paramBools(req)
		if verr != nil {
			return nil, verr
		}
		if len(bits) == 0 || len(bits) > 1968 {
			return nil, errors.New("modbus: values must contain 1..1968 coils")
		}
		raw, err = client.WriteMultipleCoils(ctx, uint16(address), uint16(len(bits)), boolsToBytes(bits))
		quantity = uint64(len(bits))
		values = bits
	default:
		return nil, fmt.Errorf("modbus: unknown action %q", req.Method)
	}
	if err != nil {
		return nil, err
	}
	out, _ := json.Marshal(map[string]any{
		"function": req.Method,
		"unit_id":  unit,
		"address":  address,
		"quantity": quantity,
		"values":   values,
		"response": hex.EncodeToString(raw),
	})
	return &apidoc.DebugResponse{
		Status:   "OK",
		Body:     out,
		Encoding: "json",
		Headers:  map[string]string{"function": req.Method, "unit_id": strconv.FormatUint(unit, 10)},
	}, nil
}

func param(req apidoc.DebugRequest, name string) string {
	for _, p := range req.Params {
		if p.Name == name && p.Value != "" {
			return p.Value
		}
	}
	return req.Options[name]
}

func paramUint(req apidoc.DebugRequest, name string, def, max uint64) (uint64, error) {
	s := strings.TrimSpace(param(req, name))
	if s == "" {
		return def, nil
	}
	v, err := strconv.ParseUint(s, 0, 64)
	if err != nil {
		return 0, fmt.Errorf("modbus: %s: %w", name, err)
	}
	if v > max {
		v = max
	}
	return v, nil
}

// paramRegisters parses a comma-separated list of decimal or 0x-prefixed
// 16-bit register values.
func paramRegisters(req apidoc.DebugRequest) ([]uint16, error) {
	s := strings.TrimSpace(param(req, "values"))
	if s == "" {
		return nil, errors.New("modbus: values is required")
	}
	var regs []uint16
	for _, part := range strings.Split(s, ",") {
		v, err := strconv.ParseUint(strings.TrimSpace(part), 0, 16)
		if err != nil {
			return nil, fmt.Errorf("modbus: values: %w", err)
		}
		regs = append(regs, uint16(v))
	}
	return regs, nil
}

// paramBools parses a comma-separated list of 0/1 coil values.
func paramBools(req apidoc.DebugRequest) ([]bool, error) {
	s := strings.TrimSpace(param(req, "values"))
	if s == "" {
		return nil, errors.New("modbus: values is required")
	}
	var bits []bool
	for _, part := range strings.Split(s, ",") {
		v, err := strconv.ParseUint(strings.TrimSpace(part), 0, 8)
		if err != nil || v > 1 {
			return nil, fmt.Errorf("modbus: values: want 0 or 1, got %q", strings.TrimSpace(part))
		}
		bits = append(bits, v == 1)
	}
	return bits, nil
}

func uint16sToBytes(regs []uint16) []byte {
	b := make([]byte, len(regs)*2)
	for i, v := range regs {
		binary.BigEndian.PutUint16(b[i*2:], v)
	}
	return b
}

func boolsToBytes(bits []bool) []byte {
	b := make([]byte, (len(bits)+7)/8)
	for i, v := range bits {
		if v {
			b[i/8] |= 1 << (i % 8)
		}
	}
	return b
}

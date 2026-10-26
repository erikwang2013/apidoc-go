//go:build apidoc_debug_industrial || apidoc_debug_all

package debug

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"time"
)

const modbusDefaultPort = "502"

func init() {
	RegisterExecutor("modbus", executeModbus)
}

var modbusExceptions = map[byte]string{
	1:  "illegal function",
	2:  "illegal data address",
	3:  "illegal data value",
	4:  "server device failure",
	5:  "acknowledge",
	6:  "server device busy",
	8:  "memory parity error",
	10: "gateway path unavailable",
	11: "gateway target device failed to respond",
}

func executeModbus(cfg Config, req *Request) (*Response, error) {
	started := time.Now()
	host, port, err := parseProtocolTarget(req.Target, "modbus", modbusDefaultPort, "modbus", "modbus-tcp", "tcp")
	if err != nil {
		return nil, err
	}
	unitID := protocolOptUint(req, "unit_id", 1)
	if unitID > 0xFF {
		return nil, fmt.Errorf("modbus unit_id must fit u8 (0-255)")
	}
	operation := req.Opts["operation"]
	if operation == "" {
		operation = "read_holding"
	}
	addressValue, found := req.Opts["address"]
	if !found {
		return nil, fmt.Errorf("modbus `address` is required")
	}
	address, err := strconv.ParseUint(addressValue, 10, 16)
	if err != nil {
		return nil, fmt.Errorf("invalid modbus address `%s`", addressValue)
	}
	quantity := uint64(1)
	if value, err := strconv.ParseUint(req.Opts["quantity"], 10, 16); err == nil {
		quantity = value
	}
	// Validate before connecting: a malformed request never reaches the device.
	switch operation {
	case "read_coils", "read_discrete":
		if err := modbusValidateRange(uint16(address), uint16(quantity), 2000); err != nil {
			return nil, err
		}
	case "read_holding", "read_input":
		if err := modbusValidateRange(uint16(address), uint16(quantity), 125); err != nil {
			return nil, err
		}
	case "write_coil", "write_register":
	default:
		return nil, fmt.Errorf("unsupported modbus operation `%s`; use read_coils, read_discrete, "+
			"read_holding, read_input, write_coil or write_register", operation)
	}

	var pdu []byte
	readBytes := 0
	switch operation {
	case "read_coils":
		pdu = modbusReadRequestPDU(1, uint16(address), uint16(quantity))
		readBytes = (int(quantity) + 7) / 8
	case "read_discrete":
		pdu = modbusReadRequestPDU(2, uint16(address), uint16(quantity))
		readBytes = (int(quantity) + 7) / 8
	case "read_holding":
		pdu = modbusReadRequestPDU(3, uint16(address), uint16(quantity))
		readBytes = int(quantity) * 2
	case "read_input":
		pdu = modbusReadRequestPDU(4, uint16(address), uint16(quantity))
		readBytes = int(quantity) * 2
	case "write_coil":
		value, err := modbusCoilValue(req.Opts["value"])
		if err != nil {
			return nil, err
		}
		encoded := uint16(0xFF00)
		if !value {
			encoded = 0
		}
		pdu = []byte{5, byte(address >> 8), byte(address), byte(encoded >> 8), byte(encoded)}
	case "write_register":
		value, err := strconv.ParseUint(req.Opts["value"], 10, 16)
		if err != nil {
			return nil, fmt.Errorf("modbus `value` is required for write_register")
		}
		pdu = []byte{6, byte(address >> 8), byte(address), byte(value >> 8), byte(value)}
	}

	conn, err := dialPinned(cfg, "tcp", host, port, req.Timeout(cfg))
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(req.Timeout(cfg))); err != nil {
		return nil, fmt.Errorf("modbus deadline failed: %v", err)
	}
	transaction := protocolRandomUint16()
	unit := byte(unitID)
	request := make([]byte, 7, 7+len(pdu))
	binary.BigEndian.PutUint16(request[0:], transaction)
	binary.BigEndian.PutUint16(request[2:], 0)
	binary.BigEndian.PutUint16(request[4:], uint16(len(pdu)+1))
	request[6] = unit
	request = append(request, pdu...)
	if len(request) > cfg.Limit() {
		return nil, fmt.Errorf("modbus request exceeds max_bytes (%d)", cfg.Limit())
	}
	if _, err := conn.Write(request); err != nil {
		return nil, fmt.Errorf("modbus write failed: %v", err)
	}
	body, err := modbusReadPDU(conn, transaction, unit, pdu[0], cfg.Limit())
	if err != nil {
		return nil, err
	}

	var values any
	switch operation {
	case "read_coils", "read_discrete":
		if len(body) < 2+readBytes {
			return nil, fmt.Errorf("modbus response truncated")
		}
		count := int(body[1])
		if count < readBytes {
			return nil, fmt.Errorf("modbus response has %d bytes, expected %d", count, readBytes)
		}
		bits := make([]bool, 0, quantity)
		for i := 0; i < int(quantity); i++ {
			bits = append(bits, body[2+i/8]&(1<<(i%8)) != 0)
		}
		values = bits
	case "read_holding", "read_input":
		if len(body) < 2+readBytes {
			return nil, fmt.Errorf("modbus response truncated")
		}
		count := int(body[1])
		if count < readBytes {
			return nil, fmt.Errorf("modbus response has %d bytes, expected %d", count, readBytes)
		}
		registers := make([]uint16, 0, quantity)
		for i := 0; i < int(quantity); i++ {
			registers = append(registers, binary.BigEndian.Uint16(body[2+i*2:]))
		}
		values = registers
	default:
		if len(body) != 5 || body[1] != pdu[1] || body[2] != pdu[2] || body[3] != pdu[3] || body[4] != pdu[4] {
			return nil, fmt.Errorf("modbus write response does not echo the request")
		}
		values = []any{}
	}
	encoded, err := json.Marshal(map[string]any{"values": values})
	if err != nil {
		return nil, fmt.Errorf("encode response: %v", err)
	}
	return &Response{
		Status: 0,
		MS:     time.Since(started).Milliseconds(),
		Headers: map[string]string{
			"x-peer":    conn.RemoteAddr().String(),
			"x-unit-id": strconv.Itoa(int(unitID)),
		},
		Body: FromBytes(encoded),
	}, nil
}

func modbusReadRequestPDU(function byte, address, quantity uint16) []byte {
	return []byte{function, byte(address >> 8), byte(address), byte(quantity >> 8), byte(quantity)}
}

// modbusReadPDU reads one MBAP-framed response and rejects transaction,
// protocol, unit or function mismatches and device exception PDUs.
func modbusReadPDU(conn net.Conn, transaction uint16, unit, function byte, limit int) ([]byte, error) {
	header := make([]byte, 7)
	if _, err := io.ReadFull(conn, header); err != nil {
		return nil, fmt.Errorf("modbus read failed: %v", err)
	}
	if binary.BigEndian.Uint16(header[0:]) != transaction {
		return nil, fmt.Errorf("modbus response transaction mismatch")
	}
	if binary.BigEndian.Uint16(header[2:]) != 0 {
		return nil, fmt.Errorf("modbus response has a non-zero protocol id")
	}
	length := int(binary.BigEndian.Uint16(header[4:]))
	if length < 2 {
		return nil, fmt.Errorf("modbus response length %d is too short", length)
	}
	if length-1 > limit {
		return nil, fmt.Errorf("modbus response exceeds max_bytes (%d)", limit)
	}
	if header[6] != unit {
		return nil, fmt.Errorf("modbus response unit mismatch")
	}
	body := make([]byte, length-1)
	if _, err := io.ReadFull(conn, body); err != nil {
		return nil, fmt.Errorf("modbus read failed: %v", err)
	}
	if body[0] == function|0x80 {
		if len(body) < 2 {
			return nil, fmt.Errorf("modbus exception response is truncated")
		}
		reason := modbusExceptions[body[1]]
		if reason == "" {
			reason = fmt.Sprintf("code %d", body[1])
		}
		return nil, fmt.Errorf("modbus exception: %s", reason)
	}
	if body[0] != function {
		return nil, fmt.Errorf("modbus response function %d does not match request %d", body[0], function)
	}
	return body, nil
}

func modbusValidateRange(address, quantity, max uint16) error {
	if quantity == 0 || quantity > max {
		return fmt.Errorf("modbus quantity must be 1..=%d, got %d", max, quantity)
	}
	if uint32(address)+uint32(quantity) > 0x10000 {
		return fmt.Errorf("modbus address range %d+%d exceeds the 16-bit address space", address, quantity)
	}
	return nil
}

func modbusCoilValue(value string) (bool, error) {
	switch strings.ToLower(value) {
	case "", "1", "true", "on":
		return true, nil
	case "0", "false", "off":
		return false, nil
	default:
		return false, fmt.Errorf("invalid coil value `%s`; use 0/1 or true/false", value)
	}
}

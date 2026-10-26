//go:build apidoc_debug_media || apidoc_debug_all

package debug

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"time"
)

var errRTMPTimeout = errors.New("rtmp session timed out")

const (
	rtmpVersion         = 3
	rtmpHandshakeSize   = 1536
	rtmpDefaultChunk    = 128
	rtmpMaxChunkStreams = 64
)

var rtmpOperations = []string{"connect", "publish", "play"}

func init() {
	RegisterExecutor("rtmp", executeRTMP)
}

func executeRTMP(cfg Config, req *Request) (*Response, error) {
	started := time.Now()
	host, port, err := parseProtocolTarget(req.Target, "rtmp", "1935", "rtmp")
	if err != nil {
		return nil, err
	}
	operation := strings.ToLower(req.Opts["operation"])
	if operation == "" {
		operation = "connect"
	}
	if !containsString(rtmpOperations, operation) {
		return nil, fmt.Errorf("unsupported rtmp operation `%s`; use connect/publish/play", operation)
	}
	conn, err := dialPinned(cfg, "tcp", host, port, req.Timeout(cfg))
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	deadline := started.Add(req.Timeout(cfg))
	if err := conn.SetDeadline(deadline); err != nil {
		return nil, fmt.Errorf("rtmp deadline failed: %v", err)
	}
	streamName, status, err := runRTMP(cfg, req, conn, host, operation, deadline)
	if err != nil {
		return nil, err
	}
	body := map[string]any{
		"connected": true,
		"operation": operation,
		"stream":    nil,
		"status":    nil,
	}
	if operation != "connect" {
		body["stream"] = streamName
	}
	if status != "" {
		body["status"] = status
	}
	encoded, err := json.Marshal(body)
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

func runRTMP(cfg Config, req *Request, conn net.Conn, host, operation string, sessionDeadline time.Time) (string, string, error) {
	if err := rtmpHandshake(conn); err != nil {
		return "", "", err
	}
	reader := newRTMPChunkReader(cfg.Limit())
	app := req.Opts["app"]
	if app == "" {
		app = "live"
	}
	props := map[string]amf0Value{
		"app":      amf0StringValue(app),
		"type":     amf0StringValue("nonprivate"),
		"flashVer": amf0StringValue("FMLE/3.0 (compatible; apidoc)"),
		"tcUrl":    amf0StringValue("rtmp://" + host + "/" + app),
	}
	if err := sendRTMPCommand(conn, 3, 0, []amf0Value{
		amf0StringValue("connect"), amf0NumberValue(1), amf0ObjectValue(props),
	}); err != nil {
		return "", "", err
	}
	if _, err := expectRTMPResult(conn, reader, 1); err != nil {
		return "", "", err
	}
	if operation == "connect" {
		return "", "", nil
	}

	streamName, found := req.Opts["stream"]
	if !found {
		return "", "", fmt.Errorf("rtmp publish/play requires opts.stream")
	}
	if err := sendRTMPCommand(conn, 3, 0, []amf0Value{
		amf0StringValue("createStream"), amf0NumberValue(2), amf0NullValue,
	}); err != nil {
		return "", "", err
	}
	result, err := expectRTMPResult(conn, reader, 2)
	if err != nil {
		return "", "", err
	}
	streamID := uint32(1)
	if len(result) > 3 && result[3].kind == amf0Number {
		streamID = uint32(result[3].number)
	}
	command := []amf0Value{
		amf0StringValue(operation), amf0NumberValue(0), amf0NullValue, amf0StringValue(streamName),
	}
	if operation == "publish" {
		command = append(command, amf0StringValue("live"))
	}
	if err := sendRTMPCommand(conn, 8, streamID, command); err != nil {
		return "", "", err
	}

	// Not every server answers publish/play, so onStatus is best effort within
	// duration_ms, but it never extends the request deadline.
	waitUntil := time.Now().Add(mediaWindow(cfg, req, 1000))
	if waitUntil.After(sessionDeadline) {
		waitUntil = sessionDeadline
	}
	status := ""
	for {
		if time.Until(waitUntil) <= 0 {
			if !time.Now().Before(sessionDeadline) {
				return "", "", errRTMPTimeout
			}
			break
		}
		if err := conn.SetReadDeadline(waitUntil); err != nil {
			return "", "", fmt.Errorf("rtmp deadline failed: %v", err)
		}
		values, err := readRTMPCommand(conn, reader)
		if err != nil {
			if errors.Is(err, errRTMPTimeout) {
				if !time.Now().Before(sessionDeadline) {
					return "", "", errRTMPTimeout
				}
				break
			}
			return "", "", err
		}
		if rtmpCommandName(values) != "onStatus" {
			continue
		}
		if level, ok := rtmpStringField(values, "level"); ok && level == "error" {
			return "", "", fmt.Errorf("%s", rtmpAMFError(values))
		}
		if code, ok := rtmpStringField(values, "code"); ok {
			status = code
		}
		break
	}
	return streamName, status, nil
}

// rtmpHandshake performs the simple (non-digest) handshake: C0+C1, S0+S1+S2,
// then C2 echoes S1.
func rtmpHandshake(conn net.Conn) error {
	c1 := make([]byte, rtmpHandshakeSize)
	copy(c1[8:], mediaRandomBytes(rtmpHandshakeSize-8))
	out := append([]byte{rtmpVersion}, c1...)
	if _, err := conn.Write(out); err != nil {
		if mediaTimeout(err) {
			return errRTMPTimeout
		}
		return fmt.Errorf("rtmp handshake write failed: %v", err)
	}
	s0, err := rtmpReadN(conn, 1)
	if err != nil {
		return err
	}
	if s0[0] != rtmpVersion {
		return fmt.Errorf("unsupported RTMP version %d (expected 3)", s0[0])
	}
	s1, err := rtmpReadN(conn, rtmpHandshakeSize)
	if err != nil {
		return err
	}
	if _, err := rtmpReadN(conn, rtmpHandshakeSize); err != nil { // S2 is not used by the simple handshake
		return err
	}
	if _, err := conn.Write(s1); err != nil {
		if mediaTimeout(err) {
			return errRTMPTimeout
		}
		return fmt.Errorf("rtmp handshake write failed: %v", err)
	}
	return nil
}

func rtmpReadN(reader io.Reader, length int) ([]byte, error) {
	buf := make([]byte, length)
	if _, err := io.ReadFull(reader, buf); err != nil {
		if mediaTimeout(err) {
			return nil, errRTMPTimeout
		}
		return nil, fmt.Errorf("rtmp read failed: %v", err)
	}
	return buf, nil
}

func rtmpReadU8(reader io.Reader) (byte, error) {
	buf, err := rtmpReadN(reader, 1)
	if err != nil {
		return 0, err
	}
	return buf[0], nil
}

func rtmpReadU24(reader io.Reader) (uint32, error) {
	buf, err := rtmpReadN(reader, 3)
	if err != nil {
		return 0, err
	}
	return uint32(buf[0])<<16 | uint32(buf[1])<<8 | uint32(buf[2]), nil
}

func rtmpReadU32LE(reader io.Reader) (uint32, error) {
	buf, err := rtmpReadN(reader, 4)
	if err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint32(buf), nil
}

// rtmpReadTimestamp consumes a 3-byte timestamp/delta and reports whether it
// used the 0xffffff extended marker. The 4-byte extended field follows the
// rest of the fmt 0/1 header, so callers consume it once the header is read.
func rtmpReadTimestamp(reader io.Reader) (bool, error) {
	buf, err := rtmpReadN(reader, 3)
	if err != nil {
		return false, err
	}
	return buf[0] == 0xff && buf[1] == 0xff && buf[2] == 0xff, nil
}

// writeRTMPMessage writes a fmt 0 message, splitting the payload at chunkSize
// with fmt 3 continuation headers.
func writeRTMPMessage(out *[]byte, csid, typeID uint8, streamID uint32, payload []byte, chunkSize int) error {
	if csid < 2 || csid > 63 {
		return fmt.Errorf("rtmp chunk stream id must be 2-63")
	}
	if len(payload) > 0xffffff {
		return fmt.Errorf("rtmp message exceeds 24-bit length")
	}
	if chunkSize < 1 {
		chunkSize = 1
	}
	*out = append(*out, csid, 0, 0, 0)
	*out = append(*out, byte(len(payload)>>16), byte(len(payload)>>8), byte(len(payload)))
	*out = append(*out, typeID)
	*out = binary.LittleEndian.AppendUint32(*out, streamID)
	for offset := 0; offset < len(payload); offset += chunkSize {
		if offset > 0 {
			*out = append(*out, 0xc0|csid) // fmt 3 continuation
		}
		end := offset + chunkSize
		if end > len(payload) {
			end = len(payload)
		}
		*out = append(*out, payload[offset:end]...)
	}
	return nil
}

type rtmpChunkState struct {
	length   int
	typeID   uint8
	streamID uint32
	extended bool
	payload  []byte
}

// rtmpChunkReader decodes fmt 0/1/2/3 chunks, extended csids and Set Chunk
// Size, with per-message and cross-csid byte caps.
type rtmpChunkReader struct {
	chunkSize int
	maxBytes  int
	states    map[uint32]*rtmpChunkState
}

func newRTMPChunkReader(maxBytes int) *rtmpChunkReader {
	return &rtmpChunkReader{
		chunkSize: rtmpDefaultChunk,
		maxBytes:  maxBytes,
		states:    map[uint32]*rtmpChunkState{},
	}
}

// readMessage returns the next command/data message; protocol control messages
// are consumed internally.
func (c *rtmpChunkReader) readMessage(reader io.Reader) (uint8, uint32, []byte, error) {
	for {
		typeID, streamID, payload, err := c.readChunkMessage(reader)
		if err != nil {
			return 0, 0, nil, err
		}
		switch typeID {
		case 1:
			if len(payload) >= 4 {
				size := binary.BigEndian.Uint32(payload[:4]) & 0x7fffffff
				if size < 1 {
					size = 1
				}
				c.chunkSize = int(size)
			}
		case 4, 5, 6: // Window Ack Size / Set Peer Bandwidth / User Control
		default:
			return typeID, streamID, payload, nil
		}
	}
}

func (c *rtmpChunkReader) readChunkMessage(reader io.Reader) (uint8, uint32, []byte, error) {
	for {
		first, err := rtmpReadU8(reader)
		if err != nil {
			return 0, 0, nil, err
		}
		format := first >> 6
		csid := uint32(first & 0x3f)
		if csid == 0 {
			value, err := rtmpReadU8(reader)
			if err != nil {
				return 0, 0, nil, err
			}
			csid = 64 + uint32(value)
		} else if csid == 1 {
			low, err := rtmpReadU8(reader)
			if err != nil {
				return 0, 0, nil, err
			}
			high, err := rtmpReadU8(reader)
			if err != nil {
				return 0, 0, nil, err
			}
			csid = 64 + uint32(low) + uint32(high)<<8
		}

		state := c.states[csid]
		delete(c.states, csid)
		switch format {
		case 0:
			extended, err := rtmpReadTimestamp(reader)
			if err != nil {
				return 0, 0, nil, err
			}
			length, err := rtmpReadU24(reader)
			if err != nil {
				return 0, 0, nil, err
			}
			typeID, err := rtmpReadU8(reader)
			if err != nil {
				return 0, 0, nil, err
			}
			streamID, err := rtmpReadU32LE(reader)
			if err != nil {
				return 0, 0, nil, err
			}
			if extended {
				if _, err := rtmpReadN(reader, 4); err != nil {
					return 0, 0, nil, err
				}
			}
			if state != nil && len(state.payload) > 0 {
				return 0, 0, nil, fmt.Errorf("rtmp new chunk header while message incomplete")
			}
			state = &rtmpChunkState{length: int(length), typeID: typeID, streamID: streamID, extended: extended}
		case 1:
			extended, err := rtmpReadTimestamp(reader)
			if err != nil {
				return 0, 0, nil, err
			}
			length, err := rtmpReadU24(reader)
			if err != nil {
				return 0, 0, nil, err
			}
			typeID, err := rtmpReadU8(reader)
			if err != nil {
				return 0, 0, nil, err
			}
			if extended {
				if _, err := rtmpReadN(reader, 4); err != nil {
					return 0, 0, nil, err
				}
			}
			if state == nil {
				return 0, 0, nil, fmt.Errorf("rtmp fmt 1 chunk without previous state")
			}
			if len(state.payload) > 0 {
				return 0, 0, nil, fmt.Errorf("rtmp new chunk header while message incomplete")
			}
			state.length, state.typeID, state.extended = int(length), typeID, extended
		case 2:
			extended, err := rtmpReadTimestamp(reader)
			if err != nil {
				return 0, 0, nil, err
			}
			if extended {
				if _, err := rtmpReadN(reader, 4); err != nil {
					return 0, 0, nil, err
				}
			}
			if state == nil {
				return 0, 0, nil, fmt.Errorf("rtmp fmt 2 chunk without previous state")
			}
			if len(state.payload) > 0 {
				return 0, 0, nil, fmt.Errorf("rtmp new chunk header while message incomplete")
			}
			state.extended = extended
		case 3:
			if state == nil {
				return 0, 0, nil, fmt.Errorf("rtmp fmt 3 chunk without state")
			}
			if state.extended {
				if _, err := rtmpReadN(reader, 4); err != nil {
					return 0, 0, nil, err
				}
			}
		}
		if state == nil {
			return 0, 0, nil, fmt.Errorf("rtmp chunk without state")
		}
		if state.length > c.maxBytes {
			return 0, 0, nil, fmt.Errorf("rtmp message exceeds max_bytes (%d)", c.maxBytes)
		}
		want := state.length - len(state.payload)
		if want > c.chunkSize {
			want = c.chunkSize
		}
		if want < 0 {
			want = 0
		}
		buffered := len(state.payload)
		for _, other := range c.states {
			buffered += len(other.payload)
		}
		if buffered+want > c.maxBytes {
			return 0, 0, nil, fmt.Errorf("rtmp cross-csid buffered data exceeds max_bytes (%d)", c.maxBytes)
		}
		data, err := rtmpReadN(reader, want)
		if err != nil {
			return 0, 0, nil, err
		}
		state.payload = append(state.payload, data...)
		if len(state.payload) == state.length {
			return state.typeID, state.streamID, state.payload, nil
		}
		if len(c.states) >= rtmpMaxChunkStreams {
			return 0, 0, nil, fmt.Errorf("too many concurrent rtmp chunk streams")
		}
		c.states[csid] = state
	}
}

func sendRTMPCommand(conn net.Conn, csid uint8, streamID uint32, values []amf0Value) error {
	payload := []byte{}
	for _, value := range values {
		if err := writeAMF0(&payload, value); err != nil {
			return err
		}
	}
	out := []byte{}
	if err := writeRTMPMessage(&out, csid, 20, streamID, payload, rtmpDefaultChunk); err != nil {
		return err
	}
	if _, err := conn.Write(out); err != nil {
		if mediaTimeout(err) {
			return errRTMPTimeout
		}
		return fmt.Errorf("rtmp write failed: %v", err)
	}
	return nil
}

func readRTMPCommand(conn net.Conn, reader *rtmpChunkReader) ([]amf0Value, error) {
	for {
		typeID, _, payload, err := reader.readMessage(conn)
		if err != nil {
			return nil, err
		}
		if typeID == 20 {
			return decodeAMF0All(payload)
		}
	}
}

func expectRTMPResult(conn net.Conn, reader *rtmpChunkReader, transaction float64) ([]amf0Value, error) {
	for {
		values, err := readRTMPCommand(conn, reader)
		if err != nil {
			return nil, err
		}
		switch rtmpCommandName(values) {
		case "_error":
			return nil, fmt.Errorf("%s", rtmpAMFError(values))
		case "_result":
			if id, ok := rtmpTransactionID(values); ok && id == transaction {
				return values, nil
			}
		}
	}
}

func rtmpCommandName(values []amf0Value) string {
	if len(values) == 0 || values[0].kind != amf0String {
		return ""
	}
	return values[0].text
}

func rtmpTransactionID(values []amf0Value) (float64, bool) {
	if len(values) < 2 || values[1].kind != amf0Number {
		return 0, false
	}
	return values[1].number, true
}

func rtmpAMFError(values []amf0Value) string {
	code, description := "unknown", ""
	for _, value := range values {
		fields := amf0Fields(value)
		if fields == nil {
			continue
		}
		if field, ok := fields["code"]; ok && field.kind == amf0String {
			code = field.text
		}
		if field, ok := fields["description"]; ok && field.kind == amf0String {
			description = field.text
		}
	}
	return fmt.Sprintf("rtmp server error %s: %s", code, description)
}

func rtmpStringField(values []amf0Value, name string) (string, bool) {
	for _, value := range values {
		fields := amf0Fields(value)
		if fields == nil {
			continue
		}
		if field, ok := fields[name]; ok && field.kind == amf0String {
			return field.text, true
		}
	}
	return "", false
}

func amf0Fields(value amf0Value) map[string]amf0Value {
	if value.kind == amf0Object || value.kind == amf0EcmaArray {
		return value.fields
	}
	return nil
}

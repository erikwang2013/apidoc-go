//go:build apidoc_debug_iot || apidoc_debug_all

package debug

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	coapDefaultPort         = "5683"
	coapContentFormatTLV    = 11542
	coapMaxMessages         = 1000
	coapTypeConfirmable     = 0
	coapTypeAcknowledgement = 2
	coapTypeReset           = 3
)

func init() {
	RegisterExecutor("coap", executeCoAP)
}

type coapOption struct {
	number uint16
	value  []byte
}

type coapPacket struct {
	packetType byte
	code       byte
	messageID  uint16
	token      []byte
	options    []coapOption
	payload    []byte
}

func coapRandomBytes(n int) []byte {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		buf = make([]byte, n) // a zero token still round-trips; never fail on entropy
	}
	return buf
}

func executeCoAP(cfg Config, req *Request) (*Response, error) {
	started := time.Now()
	host, port, err := parseProtocolTarget(req.Target, "coap", coapDefaultPort, "coap")
	if err != nil {
		return nil, err
	}
	method := req.Opts["method"]
	if method == "" {
		method = req.Method
	}
	if method == "" {
		method = "GET"
	}
	path := req.Opts["path"]
	if path == "" {
		path = "/"
	}
	contentFormat := protocolOptUint(req, "content_format", 0)
	observe, err := protocolOptBool(req, "observe", false)
	if err != nil {
		return nil, err
	}
	duration := protocolWindow(cfg, req)
	messages := 1
	if observe {
		messages = int(protocolOptUint(req, "messages", 1))
		if messages < 1 {
			messages = 1
		}
		if messages > coapMaxMessages {
			messages = coapMaxMessages
		}
	}
	payload, err := req.Body.Decode()
	if err != nil {
		return nil, err
	}
	if len(payload) > cfg.Limit() {
		return nil, fmt.Errorf("request payload exceeds max_bytes (%d)", cfg.Limit())
	}
	request, err := coapBuildRequest(method, path, nil, payload, uint32(contentFormat), observe,
		binary.BigEndian.Uint16(coapRandomBytes(2)), coapRandomBytes(8))
	if err != nil {
		return nil, err
	}
	if len(request) > cfg.Limit() {
		return nil, fmt.Errorf("coap request exceeds max_bytes (%d)", cfg.Limit())
	}
	conn, err := dialPinned(cfg, "udp", host, port, req.Timeout(cfg))
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	responses, err := coapRoundtrip(cfg, conn, request, duration, messages)
	if err != nil {
		return nil, err
	}
	items := make([]any, 0, len(responses))
	for _, packet := range responses {
		items = append(items, coapFormatPacket(packet))
	}
	var body any
	if observe && len(items) > 1 {
		body = map[string]any{"notifications": items}
	} else {
		body = items[0]
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

// coapRoundtrip sends one request and collects up to messages responses with
// a matching token, bounded by duration and the response byte cap.
func coapRoundtrip(cfg Config, conn net.Conn, request []byte, duration time.Duration, messages int) ([]coapPacket, error) {
	if _, err := conn.Write(request); err != nil {
		return nil, fmt.Errorf("coap send failed: %v", err)
	}
	requestToken := coapRequestToken(request)
	deadline := time.Now().Add(duration)
	if err := conn.SetReadDeadline(deadline); err != nil {
		return nil, fmt.Errorf("coap deadline failed: %v", err)
	}
	out := []coapPacket{}
	total := 0
	for len(out) < messages {
		buf := make([]byte, 65535)
		n, err := conn.Read(buf)
		if n > 0 {
			total += n
			if total > cfg.Limit() {
				return nil, fmt.Errorf("coap response exceeds max_bytes (%d)", cfg.Limit())
			}
			packet, decodeErr := coapDecode(buf[:n])
			if decodeErr != nil {
				continue // UDP port sharing: ignore non-CoAP datagrams
			}
			if !bytes.Equal(packet.token, requestToken) {
				continue
			}
			if packet.packetType == coapTypeReset {
				return nil, fmt.Errorf("coap peer rejected the request (RST)")
			}
			out = append(out, packet)
			continue
		}
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				break
			}
			return nil, fmt.Errorf("coap recv failed: %v", err)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("coap request timed out after %dms (no response)", duration.Milliseconds())
	}
	if coapHasObserve(request) {
		last := out[len(out)-1]
		reset := coapEncode(coapTypeReset, 0, last.messageID, requestToken, nil, nil)
		_, _ = conn.Write(reset) // best effort: the server expires Observe on its own
	}
	return out, nil
}

// coapRequestToken reads the request token after the 4-byte header.
func coapRequestToken(request []byte) []byte {
	if len(request) < 5 {
		return nil
	}
	length := int(request[0] & 0x0F)
	if 4+length > len(request) {
		return nil
	}
	return request[4 : 4+length]
}

// coapHasObserve reports whether the encoded request carries an Observe option.
func coapHasObserve(request []byte) bool {
	if len(request) < 5 {
		return false
	}
	headerLen := 4 + int(request[0]&0x0F)
	last := uint16(0)
	for pos := headerLen; pos < len(request) && request[pos] != 0xFF; {
		delta, length, used, err := coapOptionHeader(request, pos)
		if err != nil || pos+used+int(length) > len(request) {
			return false
		}
		pos += used + int(length)
		last += delta
		if last == 6 {
			return true
		}
	}
	return false
}

func coapBuildRequest(method, path string, query [][2]string, payload []byte, contentFormat uint32, observe bool, messageID uint16, token []byte) ([]byte, error) {
	var code byte
	switch strings.ToUpper(method) {
	case "GET":
		code = 1
	case "POST":
		code = 2
	case "PUT":
		code = 3
	case "DELETE":
		code = 4
	default:
		return nil, fmt.Errorf("unsupported coap method `%s`", strings.ToUpper(method))
	}
	options := []coapOption{}
	for _, segment := range strings.Split(strings.TrimPrefix(path, "/"), "/") {
		if segment != "" {
			options = append(options, coapOption{number: 11, value: []byte(segment)})
		}
	}
	if observe {
		options = append(options, coapOption{number: 6})
	}
	options = append(options, coapOption{number: 12, value: coapEncodeUint(uint64(contentFormat))})
	for _, pair := range query {
		options = append(options, coapOption{number: 15, value: []byte(pair[0] + "=" + coapPercentEncode(pair[1]))})
	}
	return coapEncode(coapTypeConfirmable, code, messageID, token, options, payload), nil
}

func coapEncode(packetType, code byte, messageID uint16, token []byte, options []coapOption, payload []byte) []byte {
	out := make([]byte, 4, 4+len(token)+len(payload))
	out[0] = 0x40 | (packetType&0x03)<<4 | byte(len(token)&0x0F)
	out[1] = code
	binary.BigEndian.PutUint16(out[2:], messageID)
	out = append(out, token...)
	sort.SliceStable(options, func(i, j int) bool { return options[i].number < options[j].number })
	last := uint16(0)
	for _, option := range options {
		delta := option.number - last
		last = option.number
		out = append(out, coapEncodeOption(delta, uint16(len(option.value)))...)
		out = append(out, option.value...)
	}
	if len(payload) > 0 {
		out = append(out, 0xFF)
		out = append(out, payload...)
	}
	return out
}

// coapEncodeOption packs the delta and length nibbles into one header byte,
// followed by their extended bytes.
func coapEncodeOption(delta, length uint16) []byte {
	deltaNibble, deltaExt := coapNibble(delta)
	lengthNibble, lengthExt := coapNibble(length)
	out := []byte{deltaNibble<<4 | lengthNibble}
	out = append(out, deltaExt...)
	return append(out, lengthExt...)
}

func coapNibble(value uint16) (byte, []byte) {
	switch {
	case value < 13:
		return byte(value), nil
	case value < 269:
		return 13, []byte{byte(value - 13)}
	default:
		return 14, []byte{byte((value - 269) >> 8), byte(value - 269)}
	}
}

// coapOptionHeader parses one option header at data[pos:]: the delta/length
// nibbles plus any extended bytes.
func coapOptionHeader(data []byte, pos int) (uint16, uint16, int, error) {
	if pos >= len(data) {
		return 0, 0, 0, fmt.Errorf("truncated coap option header")
	}
	start := pos
	header := data[pos]
	pos++
	delta, err := coapExtendedNibble(header>>4, data, &pos)
	if err != nil {
		return 0, 0, 0, err
	}
	length, err := coapExtendedNibble(header&0x0F, data, &pos)
	if err != nil {
		return 0, 0, 0, err
	}
	return delta, length, pos - start, nil
}

func coapExtendedNibble(nibble byte, data []byte, pos *int) (uint16, error) {
	switch nibble {
	case 13:
		if *pos >= len(data) {
			return 0, fmt.Errorf("truncated coap option extension")
		}
		value := uint16(data[*pos]) + 13
		*pos++
		return value, nil
	case 14:
		if *pos+2 > len(data) {
			return 0, fmt.Errorf("truncated coap option extension")
		}
		value := binary.BigEndian.Uint16(data[*pos:*pos+2]) + 269
		*pos += 2
		return value, nil
	case 15:
		return 0, fmt.Errorf("reserved coap option nibble")
	default:
		return uint16(nibble), nil
	}
}

func coapDecode(data []byte) (coapPacket, error) {
	var packet coapPacket
	if len(data) < 4 {
		return packet, fmt.Errorf("truncated coap header")
	}
	if data[0]>>6 != 1 {
		return packet, fmt.Errorf("unsupported coap version %d", data[0]>>6)
	}
	length := int(data[0] & 0x0F)
	if length > 8 || 4+length > len(data) {
		return packet, fmt.Errorf("invalid coap token length %d", length)
	}
	packet.packetType = (data[0] >> 4) & 0x03
	packet.code = data[1]
	packet.messageID = binary.BigEndian.Uint16(data[2:])
	packet.token = append([]byte{}, data[4:4+length]...)
	pos := 4 + length
	number := uint16(0)
	for pos < len(data) && data[pos] != 0xFF {
		delta, valueLen, used, err := coapOptionHeader(data, pos)
		if err != nil {
			return packet, err
		}
		pos += used
		if pos+int(valueLen) > len(data) {
			return packet, fmt.Errorf("truncated coap option value")
		}
		number += delta
		packet.options = append(packet.options, coapOption{
			number: number,
			value:  append([]byte{}, data[pos:pos+int(valueLen)]...),
		})
		pos += int(valueLen)
	}
	if pos < len(data) && data[pos] == 0xFF {
		packet.payload = append([]byte{}, data[pos+1:]...)
	}
	return packet, nil
}

func coapCodeString(code byte) string {
	class := code >> 5
	if class == 2 || class == 4 || class == 5 {
		return fmt.Sprintf("%d.%02d", class, code&0x1F)
	}
	return fmt.Sprintf("0.%02d", code)
}

func coapOptionName(number uint16) string {
	switch number {
	case 4:
		return "etag"
	case 6:
		return "observe"
	case 7:
		return "uri-port"
	case 8:
		return "location-path"
	case 12:
		return "content-format"
	case 14:
		return "max-age"
	case 15:
		return "uri-query"
	case 17:
		return "accept"
	case 20:
		return "location-query"
	default:
		return "option"
	}
}

func coapFormatOption(number uint16, value []byte) string {
	switch number {
	case 4:
		return base64.StdEncoding.EncodeToString(value)
	case 6, 7, 12, 14, 17:
		return fmt.Sprintf("%d", coapDecodeUint(value))
	default:
		if utf8Valid(value) {
			return string(value)
		}
		return base64.StdEncoding.EncodeToString(value)
	}
}

func coapFormatPacket(packet coapPacket) map[string]any {
	payload := FromBytes(packet.payload)
	grouped := map[string][]string{}
	options := map[string]string{}
	for _, option := range packet.options {
		key := coapOptionName(option.number)
		grouped[key] = append(grouped[key], coapFormatOption(option.number, option.value))
	}
	for key, values := range grouped {
		options[key] = strings.Join(values, ",")
	}
	return map[string]any{
		"code":    coapCodeString(packet.code),
		"payload": map[string]string{"encoding": payload.Encoding, "data": payload.Data},
		"options": options,
	}
}

func coapOptionUint(packet coapPacket, number uint16) uint64 {
	for _, option := range packet.options {
		if option.number == number {
			return coapDecodeUint(option.value)
		}
	}
	return 0
}

func coapOptionStrings(packet coapPacket, number uint16) []string {
	values := []string{}
	for _, option := range packet.options {
		if option.number == number {
			values = append(values, string(option.value))
		}
	}
	return values
}

// coapEncodeUint uses the shortest big-endian encoding; zero is empty.
func coapEncodeUint(value uint64) []byte {
	if value == 0 {
		return nil
	}
	out := make([]byte, 8)
	binary.BigEndian.PutUint64(out, value)
	first := 0
	for first < 7 && out[first] == 0 {
		first++
	}
	return out[first:]
}

func coapDecodeUint(value []byte) uint64 {
	var out uint64
	for _, b := range value {
		out = out<<8 | uint64(b)
	}
	return out
}

func coapPercentEncode(value string) string {
	var out strings.Builder
	for _, b := range []byte(value) {
		if (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9') ||
			b == '-' || b == '.' || b == '_' || b == '~' {
			out.WriteByte(b)
		} else {
			fmt.Fprintf(&out, "%%%02X", b)
		}
	}
	return out.String()
}

func utf8Valid(value []byte) bool { return utf8.Valid(value) }

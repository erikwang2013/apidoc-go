//go:build apidoc_debug_rpc || apidoc_debug_all

package debug

import (
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
)

const isoMaxField = 128

const (
	isoKindNumeric = iota
	isoKindAlpha
	isoKindAlphaNum
	isoKindBinary
)

type isoFraming struct {
	fixed bool
	width int // length-prefix digits for variable fields
	max   int // maximum value length in characters or hex digits
}

type isoFieldSpec struct {
	field   int
	kind    int
	framing isoFraming
}

func init() { RegisterExecutor("iso8583", executeISO8583) }

func parseISOFields(raw string) ([]isoFieldSpec, error) {
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()
	var items []any
	if err := decoder.Decode(&items); err != nil {
		return nil, fmt.Errorf("invalid iso8583 fields JSON: %v", err)
	}
	specs := make([]isoFieldSpec, 0, len(items))
	for _, item := range items {
		obj, _ := item.(map[string]any)
		fieldText, ok := isoScalarString(obj["field"])
		if !ok {
			return nil, fmt.Errorf("iso8583 field entry requires `field`")
		}
		field, err := strconv.ParseUint(fieldText, 10, 8)
		if err != nil {
			return nil, fmt.Errorf("invalid iso8583 field number")
		}
		if err := validateISOField(int(field)); err != nil {
			return nil, err
		}
		length, err := parseISOLen(obj["len"])
		if err != nil {
			return nil, err
		}
		framingName := "fixed"
		if text, ok := obj["type"].(string); ok {
			framingName = text
		}
		framing, err := parseISOFraming(framingName, length)
		if err != nil {
			return nil, err
		}
		kindName := "ans"
		if text, ok := obj["kind"].(string); ok {
			kindName = text
		}
		kind, err := parseISOKind(kindName)
		if err != nil {
			return nil, err
		}
		specs = append(specs, isoFieldSpec{field: int(field), kind: kind, framing: framing})
	}
	return finishISOSpecs(specs)
}

func isoFieldsFromParams(params []Param) ([]isoFieldSpec, error) {
	specs := make([]isoFieldSpec, 0, len(params))
	for _, param := range params {
		if strings.TrimSpace(param.Field) == "" {
			return nil, fmt.Errorf("iso8583 param `%s` requires field", param.Name)
		}
		field, err := strconv.ParseUint(strings.TrimSpace(param.Field), 10, 8)
		if err != nil {
			return nil, fmt.Errorf("invalid iso8583 field number for `%s`", param.Name)
		}
		if err := validateISOField(int(field)); err != nil {
			return nil, err
		}
		ty := strings.TrimSpace(param.Type)
		framingName, kindLen := "fixed", ty
		if idx := strings.IndexByte(ty, ':'); idx >= 0 {
			framingName, kindLen = strings.TrimSpace(ty[:idx]), strings.TrimSpace(ty[idx+1:])
		} else if idx := strings.IndexByte(ty, '.'); idx >= 0 {
			framingName, kindLen = strings.TrimSpace(ty[:idx]), strings.TrimSpace(ty[idx+1:])
		}
		kind, length, err := splitISOKindLen(kindLen)
		if err != nil {
			return nil, fmt.Errorf("iso8583 param `%s`: %v", param.Name, err)
		}
		framing, err := parseISOFraming(framingName, length)
		if err != nil {
			return nil, err
		}
		specs = append(specs, isoFieldSpec{field: int(field), kind: kind, framing: framing})
	}
	return finishISOSpecs(specs)
}

func splitISOKindLen(text string) (int, int, error) {
	digitsAt := len(text)
	for i := 0; i < len(text); i++ {
		if text[i] >= '0' && text[i] <= '9' {
			digitsAt = i
			break
		}
	}
	kind, err := parseISOKind(text[:digitsAt])
	if err != nil {
		return 0, 0, err
	}
	length, err := strconv.Atoi(text[digitsAt:])
	if err != nil || length < 1 || length > 9999 {
		return 0, 0, fmt.Errorf("ty requires a length, e.g. n6 or llvar:n19")
	}
	return kind, length, nil
}

func validateISOField(field int) error {
	if field < 2 || field > isoMaxField {
		return fmt.Errorf("iso8583 field %d is out of range 2..=%d", field, isoMaxField)
	}
	return nil
}

func isoScalarString(value any) (string, bool) {
	switch v := value.(type) {
	case string:
		return v, true
	case json.Number:
		text := string(v)
		if strings.ContainsAny(text, ".eE-") {
			return "", false
		}
		if _, err := strconv.ParseUint(text, 10, 64); err == nil {
			return text, true
		}
	}
	return "", false
}

func parseISOLen(value any) (int, error) {
	var length uint64
	ok := false
	switch v := value.(type) {
	case json.Number:
		if !strings.ContainsAny(string(v), ".eE-") {
			length, ok = parseUint(string(v))
		}
	case string:
		length, ok = parseUint(v)
	}
	if !ok {
		return 0, fmt.Errorf("iso8583 field entry requires numeric `len`")
	}
	if length < 1 || length > 9999 {
		return 0, fmt.Errorf("iso8583 field len %d is out of range 1..=9999", length)
	}
	return int(length), nil
}

func parseUint(text string) (uint64, bool) {
	value, err := strconv.ParseUint(text, 10, 64)
	return value, err == nil
}

func parseISOFraming(ty string, length int) (isoFraming, error) {
	switch strings.ToLower(strings.TrimSpace(ty)) {
	case "fixed":
		return isoFraming{fixed: true, max: length}, nil
	case "llvar":
		if length > 99 {
			return isoFraming{}, fmt.Errorf("iso8583 variable field len %d exceeds 99 for its framing", length)
		}
		return isoFraming{width: 2, max: length}, nil
	case "lllvar":
		if length > 999 {
			return isoFraming{}, fmt.Errorf("iso8583 variable field len %d exceeds 999 for its framing", length)
		}
		return isoFraming{width: 3, max: length}, nil
	case "llllvar":
		if length > 9999 {
			return isoFraming{}, fmt.Errorf("iso8583 variable field len %d exceeds 9999 for its framing", length)
		}
		return isoFraming{width: 4, max: length}, nil
	default:
		return isoFraming{}, fmt.Errorf("unsupported iso8583 field type `%s`; use fixed/llvar/lllvar/llllvar", strings.TrimSpace(ty))
	}
}

func parseISOKind(raw string) (int, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "n":
		return isoKindNumeric, nil
	case "an":
		return isoKindAlpha, nil
	case "ans":
		return isoKindAlphaNum, nil
	case "b":
		return isoKindBinary, nil
	default:
		return 0, fmt.Errorf("unsupported iso8583 field kind `%s`; use n/an/ans/b", strings.TrimSpace(raw))
	}
}

func finishISOSpecs(specs []isoFieldSpec) ([]isoFieldSpec, error) {
	sort.Slice(specs, func(i, j int) bool { return specs[i].field < specs[j].field })
	for i := 1; i < len(specs); i++ {
		if specs[i-1].field == specs[i].field {
			return nil, fmt.Errorf("duplicate iso8583 field %d", specs[i].field)
		}
	}
	for _, spec := range specs {
		if spec.kind == isoKindBinary && spec.framing.fixed && spec.framing.max%2 != 0 {
			return nil, fmt.Errorf("iso8583 binary field %d fixed len must be even", spec.field)
		}
	}
	return specs, nil
}

func executeISO8583(cfg Config, req *Request) (*Response, error) {
	started := time.Now()
	encoding := req.Opts["encoding"]
	if encoding == "" {
		encoding = "ascii"
	}
	if encoding != "ascii" {
		return nil, fmt.Errorf("iso8583 encoding `%s` is not supported in this build; only ascii (ebcdic is P6)", encoding)
	}
	var specs []isoFieldSpec
	var err error
	if raw := req.Opts["fields"]; raw != "" {
		specs, err = parseISOFields(raw)
	} else if req.Endpoint != nil && len(req.Endpoint.Params) > 0 {
		specs, err = isoFieldsFromParams(req.Endpoint.Params)
	} else {
		err = fmt.Errorf("iso8583 requires opts.fields JSON or endpoint params with field/ty")
	}
	if err != nil {
		return nil, err
	}

	body, err := decodeJSONBody(req)
	if err != nil {
		return nil, fmt.Errorf("invalid iso8583 request JSON: %v", err)
	}
	fields, err := isoBodyFields(body)
	if err != nil {
		return nil, err
	}
	mti := req.Opts["mti"]
	if mti == "" {
		bodyObj, _ := body.(map[string]any)
		if text, ok := bodyObj["mti"].(string); ok {
			mti = text
		}
	}
	if mti == "" {
		return nil, fmt.Errorf("iso8583 requires opts.mti (e.g. \"0200\")")
	}
	message, err := encodeISOMessage(mti, specs, fields)
	if err != nil {
		return nil, err
	}
	if len(message) > cfg.Limit() {
		return nil, fmt.Errorf("iso8583 request exceeds max_bytes (%d)", cfg.Limit())
	}

	framing := req.Opts["framing"]
	if framing == "" {
		framing = "2"
	}
	width := 0
	switch framing {
	case "2":
		width = 2
	case "4":
		width = 4
	case "none":
	default:
		return nil, fmt.Errorf("unsupported iso8583 framing `%s`; use 2, 4 or none", framing)
	}
	if width == 2 && len(message) > 0xffff {
		return nil, fmt.Errorf("iso8583 request exceeds 2-byte framing length")
	}

	host, port, err := rpcAddr(req.Target, "tcp")
	if err != nil {
		return nil, err
	}
	addrs, err := CheckHost(cfg, host, port)
	if err != nil {
		return nil, err
	}
	peer := net.JoinHostPort(addrs[0].String(), port)
	conn, err := dialRPC(cfg, req, "iso8583", "tcp", peer)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	frame := []byte{}
	switch width {
	case 2:
		frame = binary.BigEndian.AppendUint16(frame, uint16(len(message)))
	case 4:
		frame = binary.BigEndian.AppendUint32(frame, uint32(len(message)))
	}
	frame = append(frame, message...)
	_ = conn.SetDeadline(time.Now().Add(req.Timeout(cfg)))
	if _, err := conn.Write(frame); err != nil {
		return nil, isoNetError(err, "iso8583 write failed")
	}
	var response []byte
	if width == 0 {
		response, err = readISOUntilParsed(cfg, req, conn, specs)
	} else {
		response, err = readISOFramed(cfg, conn, width)
	}
	if err != nil {
		return nil, err
	}
	value, err := decodeISOMessage(response, specs)
	if err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("iso8583 response encode failed: %v", err)
	}
	return &Response{
		Status: 0,
		MS:     time.Since(started).Milliseconds(),
		Headers: map[string]string{
			"x-peer": peer,
			"x-mti":  value["mti"].(string),
		},
		Body: Payload{Encoding: "utf8", Data: string(encoded)},
	}, nil
}

func isoBodyFields(body any) (map[string]any, error) {
	obj, ok := body.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("iso8583 request body must be a JSON object")
	}
	if value, ok := obj["fields"]; ok {
		fields, ok := value.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("iso8583 body.fields must be an object")
		}
		return fields, nil
	}
	return obj, nil
}

func encodeISOMessage(mti string, specs []isoFieldSpec, body map[string]any) ([]byte, error) {
	if len(mti) != 4 || !isoDigits(mti) {
		return nil, fmt.Errorf("invalid iso8583 MTI `%s`; expected 4 ASCII digits", mti)
	}
	values := map[int]any{}
	for key, value := range body {
		field, err := strconv.ParseUint(key, 10, 8)
		if err != nil {
			return nil, fmt.Errorf("invalid iso8583 field number `%s`", key)
		}
		values[int(field)] = value
	}
	configured := map[int]bool{}
	for _, spec := range specs {
		configured[spec.field] = true
	}
	hasSecondary := false
	for field := range values {
		if !configured[field] {
			return nil, fmt.Errorf("iso8583 field %d is set but not configured", field)
		}
		if field > 64 {
			hasSecondary = true
		}
	}
	primary := [8]byte{}
	secondary := [8]byte{}
	if hasSecondary {
		setISOBit(&primary, 1)
	}
	for _, spec := range specs {
		if _, ok := values[spec.field]; !ok {
			continue
		}
		if spec.field <= 64 {
			setISOBit(&primary, spec.field)
		} else {
			setISOBit(&secondary, spec.field-64)
		}
	}

	out := []byte(mti)
	out = append(out, primary[:]...)
	if hasSecondary {
		out = append(out, secondary[:]...)
	}
	for _, spec := range specs {
		if value, ok := values[spec.field]; ok {
			var err error
			out, err = encodeISOField(out, spec, value)
			if err != nil {
				return nil, err
			}
		}
	}
	return out, nil
}

func decodeISOMessage(bytes []byte, specs []isoFieldSpec) (map[string]any, error) {
	r := &isoReader{data: bytes}
	mtiBytes, err := r.take(4)
	if err != nil {
		return nil, err
	}
	mti := string(mtiBytes)
	if !isoDigits(mti) {
		return nil, fmt.Errorf("invalid iso8583 MTI `%s`", mti)
	}
	primaryBytes, err := r.take(8)
	if err != nil {
		return nil, err
	}
	primary := [8]byte{}
	copy(primary[:], primaryBytes)
	secondary := [8]byte{}
	hasSecondary := primary[0]&0x80 != 0
	if hasSecondary {
		secondaryBytes, err := r.take(8)
		if err != nil {
			return nil, err
		}
		copy(secondary[:], secondaryBytes)
	}

	set := map[int]bool{}
	for field := 2; field <= 64; field++ {
		if isoBit(&primary, field) {
			set[field] = true
		}
	}
	if hasSecondary {
		for field := 65; field <= isoMaxField; field++ {
			if isoBit(&secondary, field-64) {
				set[field] = true
			}
		}
	}
	configured := map[int]bool{}
	for _, spec := range specs {
		configured[spec.field] = true
	}
	for field := range set {
		if !configured[field] {
			return nil, fmt.Errorf("iso8583 field %d is set but not configured", field)
		}
	}

	fields := map[string]any{}
	for _, spec := range specs {
		if !set[spec.field] {
			continue
		}
		value, err := decodeISOField(r, spec)
		if err != nil {
			return nil, err
		}
		fields[strconv.Itoa(spec.field)] = value
	}
	if r.off != len(r.data) {
		return nil, fmt.Errorf("iso8583: %d trailing byte(s) after last field", len(r.data)-r.off)
	}
	return map[string]any{"mti": mti, "fields": fields}, nil
}

func encodeISOField(out []byte, spec isoFieldSpec, value any) ([]byte, error) {
	text, ok := isoScalarString(value)
	if !ok {
		return nil, fmt.Errorf("iso8583 field %d must be a string or number", spec.field)
	}
	if spec.kind == isoKindBinary {
		hexText := normalizeISOHex(text)
		if !isoHexDigits(hexText) {
			return nil, fmt.Errorf("iso8583 binary field requires hex digits")
		}
		return encodeISOBinary(out, spec, hexText)
	}
	if err := validateISOText(spec, text); err != nil {
		return nil, err
	}
	if len(text) > spec.framing.max {
		return nil, fmt.Errorf("iso8583 field %d is %d chars, max is %d", spec.field, len(text), spec.framing.max)
	}
	if spec.framing.fixed {
		length := spec.framing.max
		if spec.kind == isoKindNumeric {
			out = append(out, strings.Repeat("0", length-len(text))...)
			return append(out, text...), nil
		}
		out = append(out, text...)
		return append(out, strings.Repeat(" ", length-len(text))...), nil
	}
	out = append(out, fmt.Sprintf("%0*d", spec.framing.width, len(text))...)
	return append(out, text...), nil
}

func decodeISOField(r *isoReader, spec isoFieldSpec) (string, error) {
	length := spec.framing.max
	if !spec.framing.fixed {
		prefix, err := r.take(spec.framing.width)
		if err != nil {
			return "", err
		}
		text := string(prefix)
		if !isoDigits(text) {
			return "", fmt.Errorf("iso8583: invalid length prefix")
		}
		length, err = strconv.Atoi(text)
		if err != nil {
			return "", fmt.Errorf("iso8583: invalid length prefix")
		}
	}
	if length > spec.framing.max {
		return "", fmt.Errorf("iso8583 field %d length %d exceeds configured max", spec.field, length)
	}
	if spec.kind == isoKindBinary {
		if length%2 != 0 {
			return "", fmt.Errorf("iso8583 binary field %d has odd digit length %d", spec.field, length)
		}
		data, err := r.take(length / 2)
		if err != nil {
			return "", err
		}
		return strings.ToUpper(hex.EncodeToString(data)), nil
	}
	data, err := r.take(length)
	if err != nil {
		return "", err
	}
	text := string(data)
	if !isoASCII(text) {
		return "", fmt.Errorf("iso8583 field %d is not ASCII", spec.field)
	}
	if spec.kind == isoKindNumeric {
		return text, nil
	}
	return strings.TrimRightFunc(text, unicode.IsSpace), nil
}

func encodeISOBinary(out []byte, spec isoFieldSpec, hexText string) ([]byte, error) {
	if len(hexText) > spec.framing.max {
		return nil, fmt.Errorf("iso8583 field %d is %d hex digits, max is %d", spec.field, len(hexText), spec.framing.max)
	}
	if spec.framing.fixed {
		hexText = strings.Repeat("0", spec.framing.max-len(hexText)) + hexText
	} else {
		if len(hexText)%2 != 0 {
			return nil, fmt.Errorf("iso8583 binary field %d requires an even number of hex digits", spec.field)
		}
		out = append(out, fmt.Sprintf("%0*d", spec.framing.width, len(hexText))...)
	}
	raw, err := hex.DecodeString(hexText)
	if err != nil {
		return nil, fmt.Errorf("invalid iso8583 hex")
	}
	return append(out, raw...), nil
}

func normalizeISOHex(text string) string {
	return strings.ToUpper(strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, text))
}

func isoHexDigits(text string) bool {
	for i := 0; i < len(text); i++ {
		c := text[i]
		if !(c >= '0' && c <= '9' || c >= 'A' && c <= 'F' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

func validateISOText(spec isoFieldSpec, text string) error {
	switch spec.kind {
	case isoKindNumeric:
		if !isoDigits(text) {
			return fmt.Errorf("iso8583 field %d requires digits", spec.field)
		}
	case isoKindAlpha, isoKindAlphaNum:
		if !isoASCII(text) {
			return fmt.Errorf("iso8583 field %d requires ASCII text", spec.field)
		}
	}
	return nil
}

func isoDigits(text string) bool {
	for i := 0; i < len(text); i++ {
		if text[i] < '0' || text[i] > '9' {
			return false
		}
	}
	return true
}

func isoASCII(text string) bool {
	for i := 0; i < len(text); i++ {
		if text[i] > 0x7f {
			return false
		}
	}
	return true
}

func setISOBit(bitmap *[8]byte, field int) {
	idx := field - 1
	bitmap[idx/8] |= 0x80 >> (idx % 8)
}

func isoBit(bitmap *[8]byte, field int) bool {
	idx := field - 1
	return bitmap[idx/8]&(0x80>>(idx%8)) != 0
}

type isoReader struct {
	data []byte
	off  int
}

func (r *isoReader) take(length int) ([]byte, error) {
	if len(r.data)-r.off < length {
		return nil, errIncomplete
	}
	out := r.data[r.off : r.off+length]
	r.off += length
	return out, nil
}

func readISOFramed(cfg Config, conn net.Conn, width int) ([]byte, error) {
	head := make([]byte, width)
	if _, err := io.ReadFull(conn, head); err != nil {
		return nil, isoNetError(err, "iso8583 read length failed")
	}
	length := 0
	if width == 2 {
		length = int(binary.BigEndian.Uint16(head))
	} else {
		length = int(binary.BigEndian.Uint32(head))
	}
	if length > cfg.Limit() {
		return nil, fmt.Errorf("iso8583 response exceeds max_bytes (%d)", cfg.Limit())
	}
	body := make([]byte, length)
	if _, err := io.ReadFull(conn, body); err != nil {
		return nil, isoNetError(err, "iso8583 read body failed")
	}
	return body, nil
}

func readISOUntilParsed(cfg Config, req *Request, conn net.Conn, specs []isoFieldSpec) ([]byte, error) {
	deadline := time.Now().Add(rpcReadWindow(cfg, req))
	buf := []byte{}
	chunk := make([]byte, 4096)
	for {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			break
		}
		if err := conn.SetReadDeadline(time.Now().Add(remaining)); err != nil {
			return nil, fmt.Errorf("iso8583 read failed: %v", err)
		}
		n, err := conn.Read(chunk)
		if n > 0 {
			buf = append(buf, chunk[:n]...)
			if len(buf) > cfg.Limit() {
				return nil, fmt.Errorf("iso8583 response exceeds max_bytes (%d)", cfg.Limit())
			}
			if _, decodeErr := decodeISOMessage(buf, specs); decodeErr == nil {
				return buf, nil
			} else if !errors.Is(decodeErr, errIncomplete) {
				return nil, decodeErr
			}
		}
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				break
			}
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, fmt.Errorf("iso8583 read failed: %v", err)
		}
	}
	if len(buf) == 0 {
		return nil, fmt.Errorf("iso8583 response timed out")
	}
	return nil, fmt.Errorf("incomplete iso8583 response")
}

func isoNetError(err error, label string) error {
	if ne, ok := err.(net.Error); ok && ne.Timeout() {
		return fmt.Errorf("iso8583 request timed out")
	}
	return fmt.Errorf("%s: %v", label, err)
}

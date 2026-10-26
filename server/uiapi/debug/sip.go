//go:build apidoc_debug_media || apidoc_debug_all

package debug

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"
)

// sipMethods is the single-transaction subset; dialogs and media stay out of
// scope.
var sipMethods = []string{"OPTIONS", "REGISTER", "INVITE", "MESSAGE", "BYE"}

const sipMaxBody = 65535

func init() {
	RegisterExecutor("sip", executeSIP)
}

type sipResponse struct {
	status  int
	reason  string
	headers map[string]string
	body    []byte
}

func executeSIP(cfg Config, req *Request) (*Response, error) {
	started := time.Now()
	method := strings.ToUpper(req.Opts["method"])
	if method == "" {
		method = "OPTIONS"
	}
	if !containsString(sipMethods, method) {
		return nil, fmt.Errorf("unsupported SIP method `%s`; use %s (dialogs/media are out of scope)",
			method, strings.Join(sipMethods, "/"))
	}
	transport := strings.ToLower(req.Opts["transport"])
	if transport == "" {
		transport = "udp"
	}
	if transport != "udp" && transport != "tcp" {
		return nil, fmt.Errorf("unsupported SIP transport `%s`; use udp or tcp", transport)
	}
	host, port, err := parseProtocolTarget(req.Target, "sip", "5060", "sip")
	if err != nil {
		return nil, err
	}
	conn, err := dialPinned(cfg, transport, host, port, req.Timeout(cfg))
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	if err := conn.SetDeadline(started.Add(req.Timeout(cfg))); err != nil {
		return nil, fmt.Errorf("SIP deadline failed: %v", err)
	}
	request, callID, cseq, err := buildSIPRequest(req, method, host, conn.LocalAddr(), transport, cfg)
	if err != nil {
		return nil, err
	}

	var response *sipResponse
	if transport == "tcp" {
		if _, err := conn.Write(request); err != nil {
			if mediaTimeout(err) {
				return nil, fmt.Errorf("SIP transaction timed out")
			}
			return nil, fmt.Errorf("SIP tcp send failed: %v", err)
		}
		response, err = sipTCPExchange(conn, cfg.Limit(), callID, method, cseq)
	} else {
		response, err = sipUDPExchange(cfg, conn, request, callID, method, cseq, started.Add(req.Timeout(cfg)))
	}
	if err != nil {
		if mediaTimeout(err) {
			return nil, fmt.Errorf("SIP transaction timed out")
		}
		return nil, err
	}
	if response.status == 401 || response.status == 407 {
		label := "401 Unauthorized"
		if response.status == 407 {
			label = "407 Proxy Authentication Required"
		}
		return nil, fmt.Errorf("SIP %s: Digest authentication is required but credentials-based auth is not supported by the debug executor", label)
	}
	body, err := json.Marshal(map[string]any{
		"status":  response.status,
		"reason":  response.reason,
		"headers": response.headers,
		"body":    mediaPayload(response.body),
	})
	if err != nil {
		return nil, fmt.Errorf("encode response: %v", err)
	}
	return &Response{
		Status:  response.status,
		MS:      time.Since(started).Milliseconds(),
		Headers: response.headers,
		Body:    FromBytes(body),
	}, nil
}

func buildSIPRequest(req *Request, method, host string, local net.Addr, transport string, cfg Config) ([]byte, string, uint64, error) {
	uriHost := host
	if strings.Contains(host, ":") {
		uriHost = "[" + host + "]"
	}
	uri := req.Opts["uri"]
	if uri == "" {
		uri = "sip:" + uriHost
	}
	from := req.Opts["from"]
	if from == "" {
		from = "sip:apidoc@" + uriHost
	}
	to := req.Opts["to"]
	if to == "" {
		to = uri
	}
	branch := req.Opts["branch"]
	if branch == "" {
		branch = "z9hG4bK" + mediaRandomHex(16)
	}
	callID := req.Opts["call_id"]
	if callID == "" {
		callID = mediaRandomHex(16) + "@apidoc"
	}
	cseq := protocolOptUint(req, "cseq", 1)
	tag := "apidoc" + mediaRandomHex(8)
	for name, value := range map[string]string{
		"uri": uri, "from": from, "to": to, "branch": branch, "call_id": callID,
	} {
		if strings.ContainsAny(value, "\r\n") {
			return nil, "", 0, fmt.Errorf("SIP `%s` must not contain CR/LF", name)
		}
	}
	body, err := req.Body.Decode()
	if err != nil {
		return nil, "", 0, err
	}
	if len(body) > sipMaxBody {
		return nil, "", 0, fmt.Errorf("SIP body exceeds %d bytes", sipMaxBody)
	}
	if len(body) > cfg.Limit() {
		return nil, "", 0, fmt.Errorf("request body exceeds max_bytes (%d)", cfg.Limit())
	}
	if len(body) > 0 && !utf8.Valid(body) {
		return nil, "", 0, fmt.Errorf("SIP body must be valid UTF-8 (SDP/plain text)")
	}

	localHost, localPort := sipLocalAddr(local)
	localAddr := net.JoinHostPort(localHost, localPort)
	transportUpper := strings.ToUpper(transport)
	type header struct{ name, value string }
	headers := []header{
		{"Via", fmt.Sprintf("SIP/2.0/%s %s;branch=%s;rport", transportUpper, localAddr, branch)},
		{"Max-Forwards", "70"},
		{"From", "<" + from + ">;tag=" + tag},
		{"To", "<" + to + ">"},
		{"Call-ID", callID},
		{"CSeq", fmt.Sprintf("%d %s", cseq, method)},
		{"Contact", fmt.Sprintf("<sip:apidoc@%s;transport=%s>", localAddr, transport)},
	}
	managed := map[string]bool{
		"via": true, "max-forwards": true, "from": true, "to": true,
		"call-id": true, "cseq": true, "contact": true, "content-length": true,
	}
	names := make([]string, 0, len(req.Headers))
	for name := range req.Headers {
		if !managed[strings.ToLower(name)] {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	hasContentType := false
	for _, name := range names {
		hasContentType = hasContentType || strings.EqualFold(name, "content-type")
		headers = append(headers, header{name, req.Headers[name]})
	}
	if len(body) > 0 && !hasContentType {
		headers = append(headers, header{"Content-Type", "application/sdp"})
	}
	headers = append(headers, header{"Content-Length", strconv.Itoa(len(body))})

	var out bytes.Buffer
	out.WriteString(method + " " + uri + " SIP/2.0\r\n")
	for _, item := range headers {
		if item.name == "" || strings.ContainsAny(item.name, ":\r\n") {
			return nil, "", 0, fmt.Errorf("invalid SIP header name `%s`", item.name)
		}
		if strings.ContainsAny(item.value, "\r\n") {
			return nil, "", 0, fmt.Errorf("SIP header `%s` must not contain CR/LF", item.name)
		}
		out.WriteString(item.name + ": " + item.value + "\r\n")
	}
	out.WriteString("\r\n")
	out.Write(body)
	request := out.Bytes()
	if len(request) > cfg.Limit() {
		return nil, "", 0, fmt.Errorf("SIP request exceeds max_bytes (%d)", cfg.Limit())
	}
	return request, callID, cseq, nil
}

func sipLocalAddr(addr net.Addr) (string, string) {
	switch value := addr.(type) {
	case *net.TCPAddr:
		return value.IP.String(), strconv.Itoa(value.Port)
	case *net.UDPAddr:
		return value.IP.String(), strconv.Itoa(value.Port)
	}
	if host, port, err := net.SplitHostPort(addr.String()); err == nil {
		return host, port
	}
	return addr.String(), "0"
}

// sipUDPExchange retransmits per RFC 3261 timers (500ms/1s/2s) and only a
// matching final response completes the transaction.
func sipUDPExchange(cfg Config, conn net.Conn, request []byte, callID, method string, cseq uint64, deadline time.Time) (*sipResponse, error) {
	buf := make([]byte, 65535)
	for attempt := 0; attempt < 4; attempt++ {
		if _, err := conn.Write(request); err != nil {
			if mediaTimeout(err) {
				break
			}
			return nil, fmt.Errorf("SIP udp send failed: %v", err)
		}
		remaining := time.Until(deadline)
		if remaining <= 0 {
			break
		}
		wait := remaining
		if attempt < 3 {
			if timer := time.Duration(500<<attempt) * time.Millisecond; timer < wait {
				wait = timer
			}
		}
		waitUntil := time.Now().Add(wait)
		for {
			left := time.Until(waitUntil)
			if left <= 0 {
				break
			}
			if err := conn.SetReadDeadline(waitUntil); err != nil {
				return nil, fmt.Errorf("SIP udp deadline failed: %v", err)
			}
			n, err := conn.Read(buf)
			if err != nil {
				// Timeouts end this attempt; ICMP refusals also just mean no
				// response, so the next attempt still applies.
				if mediaTimeout(err) || errors.Is(err, syscall.ECONNREFUSED) {
					break
				}
				return nil, fmt.Errorf("SIP udp recv failed: %v", err)
			}
			if n > cfg.Limit() {
				return nil, fmt.Errorf("SIP response exceeds max_bytes (%d)", cfg.Limit())
			}
			if n == len(buf) {
				return nil, fmt.Errorf("SIP response exceeds a single %d-byte datagram", len(buf))
			}
			response, err := parseSIPResponse(buf[:n])
			if err != nil {
				continue
			}
			if isFinalSIP(response, callID, cseq, method) {
				return response, nil
			}
		}
	}
	return nil, fmt.Errorf("SIP transaction timed out waiting for a final response matching Call-ID %s CSeq %d", callID, cseq)
}

func sipTCPExchange(conn net.Conn, maxBytes int, callID, method string, cseq uint64) (*sipResponse, error) {
	buffer := []byte{}
	for {
		message, rest, err := readSIPMessage(conn, buffer, maxBytes)
		if err != nil {
			return nil, err
		}
		buffer = rest
		response, err := parseSIPResponse(message)
		if err != nil {
			return nil, fmt.Errorf("SIP parse failed: %v", err)
		}
		if isFinalSIP(response, callID, cseq, method) {
			return response, nil
		}
	}
}

func readSIPMessage(conn net.Conn, buffer []byte, maxBytes int) ([]byte, []byte, error) {
	for {
		if pos := bytes.Index(buffer, []byte("\r\n\r\n")); pos >= 0 {
			length := uint64(0)
			for _, line := range strings.Split(string(buffer[:pos]), "\r\n") {
				name, value, found := strings.Cut(line, ":")
				if found && strings.EqualFold(strings.TrimSpace(name), "content-length") {
					length, _ = strconv.ParseUint(strings.TrimSpace(value), 10, 63)
				}
			}
			if pos+4 > maxBytes || length > uint64(maxBytes-pos-4) {
				return nil, nil, fmt.Errorf("SIP response exceeds max_bytes (%d)", maxBytes)
			}
			total := pos + 4 + int(length)
			for len(buffer) < total {
				chunk := make([]byte, 4096)
				n, err := conn.Read(chunk)
				if n > 0 {
					buffer = append(buffer, chunk[:n]...)
				}
				if err != nil {
					if mediaTimeout(err) {
						return nil, nil, err
					}
					return nil, nil, fmt.Errorf("SIP TCP connection closed before Content-Length was read")
				}
				if n == 0 {
					return nil, nil, fmt.Errorf("SIP TCP connection closed before Content-Length was read")
				}
			}
			return append([]byte{}, buffer[:total]...), append([]byte{}, buffer[total:]...), nil
		}
		if len(buffer) > maxBytes {
			return nil, nil, fmt.Errorf("SIP response exceeds max_bytes (%d)", maxBytes)
		}
		chunk := make([]byte, 4096)
		n, err := conn.Read(chunk)
		if n > 0 {
			buffer = append(buffer, chunk[:n]...)
		}
		if err != nil {
			if mediaTimeout(err) {
				return nil, nil, err
			}
			if errors.Is(err, io.EOF) {
				return nil, nil, fmt.Errorf("SIP TCP connection closed before a complete message")
			}
			return nil, nil, fmt.Errorf("SIP TCP read failed: %v", err)
		}
		if n == 0 {
			return nil, nil, fmt.Errorf("SIP TCP connection closed before a complete message")
		}
	}
}

func parseSIPResponse(raw []byte) (*sipResponse, error) {
	pos := bytes.Index(raw, []byte("\r\n\r\n"))
	if pos < 0 {
		return nil, fmt.Errorf("truncated SIP message")
	}
	lines := strings.Split(string(raw[:pos]), "\r\n")
	parts := strings.SplitN(lines[0], " ", 3)
	if len(parts) < 2 || !strings.HasPrefix(parts[0], "SIP/") {
		return nil, fmt.Errorf("invalid SIP status line `%s`", lines[0])
	}
	status, err := strconv.Atoi(parts[1])
	if err != nil {
		return nil, fmt.Errorf("invalid SIP status line `%s`", lines[0])
	}
	reason := ""
	if len(parts) == 3 {
		reason = parts[2]
	}
	headers := map[string]string{}
	for _, line := range lines[1:] {
		name, value, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		name, value = strings.TrimSpace(name), strings.TrimSpace(value)
		if existing, ok := headers[name]; ok {
			headers[name] = existing + ", " + value
		} else {
			headers[name] = value
		}
	}
	body := raw[pos+4:]
	if value, ok := mediaHeader(headers, "content-length"); ok {
		length, err := strconv.ParseUint(strings.TrimSpace(value), 10, 63)
		if err != nil {
			return nil, fmt.Errorf("invalid SIP Content-Length `%s`", value)
		}
		if length > uint64(len(body)) {
			return nil, fmt.Errorf("SIP Content-Length %d exceeds received body (%d bytes)", length, len(body))
		}
		body = body[:int(length)]
	}
	return &sipResponse{
		status:  status,
		reason:  reason,
		headers: headers,
		body:    append([]byte{}, body...),
	}, nil
}

// isFinalSIP requires 2xx-6xx plus a matching Call-ID and CSeq; 1xx keeps the
// transaction open.
func isFinalSIP(response *sipResponse, callID string, cseq uint64, method string) bool {
	if response.status < 200 {
		return false
	}
	headerCallID, ok := mediaHeader(response.headers, "call-id")
	if !ok || strings.TrimSpace(headerCallID) != callID {
		return false
	}
	value, ok := mediaHeader(response.headers, "cseq")
	if !ok {
		return false
	}
	parts := strings.Fields(value)
	if len(parts) == 0 {
		return false
	}
	number, err := strconv.ParseUint(parts[0], 10, 64)
	if err != nil || number != cseq {
		return false
	}
	return len(parts) == 1 || strings.EqualFold(parts[1], method)
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

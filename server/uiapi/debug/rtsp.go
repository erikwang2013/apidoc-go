//go:build apidoc_debug_media || apidoc_debug_all

package debug

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
)

var rtspMethods = []string{"OPTIONS", "DESCRIBE", "SETUP", "PLAY", "PAUSE", "TEARDOWN"}

func init() {
	RegisterExecutor("rtsp", executeRTSP)
}

func executeRTSP(cfg Config, req *Request) (*Response, error) {
	started := time.Now()
	url := rtspRequestURL(req)
	if !strings.HasPrefix(url, "rtsp://") {
		return nil, fmt.Errorf("rtsp target must use the rtsp:// scheme")
	}
	if strings.ContainsAny(url, "\r\n ") {
		return nil, fmt.Errorf("rtsp URL must not contain whitespace or CR/LF")
	}
	host, port, err := parseProtocolTarget(url, "rtsp", "554", "rtsp")
	if err != nil {
		return nil, err
	}
	method := strings.ToUpper(req.Opts["method"])
	if method == "" {
		method = "OPTIONS"
	}
	if !slices.Contains(rtspMethods, method) {
		return nil, fmt.Errorf("unsupported rtsp method `%s`; use %s", method, strings.Join(rtspMethods, "/"))
	}
	cseq := protocolOptUint(req, "cseq", 1)
	body, err := req.Body.Decode()
	if err != nil {
		return nil, err
	}
	if len(body) > cfg.Limit() {
		return nil, fmt.Errorf("request body exceeds max_bytes (%d)", cfg.Limit())
	}
	request, err := buildRTSPRequest(req, method, url, cseq, body)
	if err != nil {
		return nil, err
	}

	conn, err := dialPinned(cfg, "tcp", host, port, req.Timeout(cfg))
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	if err := conn.SetDeadline(started.Add(req.Timeout(cfg))); err != nil {
		return nil, fmt.Errorf("rtsp deadline failed: %v", err)
	}
	if _, err := conn.Write(request); err != nil {
		if mediaTimeout(err) {
			return nil, fmt.Errorf("rtsp request timed out")
		}
		return nil, fmt.Errorf("rtsp write failed: %v", err)
	}
	status, reason, headers, responseBody, err := readRTSPResponse(conn, cfg.Limit())
	if err != nil {
		if mediaTimeout(err) {
			return nil, fmt.Errorf("rtsp request timed out")
		}
		return nil, err
	}
	if status == 401 {
		return nil, rtspAuthError("401 Unauthorized", headers, "WWW-Authenticate")
	}
	if status == 407 {
		return nil, rtspAuthError("407 Proxy Authentication Required", headers, "Proxy-Authenticate")
	}
	responseCSeq, _ := mediaHeader(headers, "cseq")
	if responseCSeq != strconv.FormatUint(cseq, 10) {
		return nil, fmt.Errorf("rtsp response CSeq `%s` does not match request CSeq %d", responseCSeq, cseq)
	}
	headers["x-reason"] = reason
	return &Response{
		Status:  status,
		MS:      time.Since(started).Milliseconds(),
		Headers: headers,
		Body:    FromBytes(responseBody),
	}, nil
}

func rtspRequestURL(req *Request) string {
	if url, found := req.Opts["url"]; found {
		return url
	}
	if strings.Contains(req.Target, "://") {
		return req.Target
	}
	return "rtsp://" + req.Target
}

func buildRTSPRequest(req *Request, method, url string, cseq uint64, body []byte) ([]byte, error) {
	type header struct{ name, value string }
	headers := []header{
		{"CSeq", strconv.FormatUint(cseq, 10)},
		{"User-Agent", "apidoc-go"},
	}
	if session := req.Opts["session"]; session != "" {
		headers = append(headers, header{"Session", session})
	}
	if method == "SETUP" {
		transport := req.Opts["transport"]
		if transport == "" {
			transport = "RTP/AVP/TCP;interleaved=0-1"
		}
		headers = append(headers, header{"Transport", transport})
	}
	managed := map[string]bool{"cseq": true, "content-length": true, "user-agent": true}
	names := make([]string, 0, len(req.Headers))
	for name := range req.Headers {
		if !managed[strings.ToLower(name)] {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	hasContentType := false
	for _, name := range names {
		value := req.Headers[name]
		hasContentType = hasContentType || strings.EqualFold(name, "content-type")
		headers = append(headers, header{name, value})
	}
	if len(body) > 0 && !hasContentType {
		headers = append(headers, header{"Content-Type", "application/sdp"})
	}
	headers = append(headers, header{"Content-Length", strconv.Itoa(len(body))})

	var out bytes.Buffer
	out.WriteString(method + " " + url + " RTSP/1.0\r\n")
	for _, item := range headers {
		if item.name == "" || strings.ContainsAny(item.name, ":\r\n") {
			return nil, fmt.Errorf("invalid rtsp header name `%s`", item.name)
		}
		if strings.ContainsAny(item.value, "\r\n") {
			return nil, fmt.Errorf("rtsp header `%s` must not contain CR/LF", item.name)
		}
		out.WriteString(item.name + ": " + item.value + "\r\n")
	}
	out.WriteString("\r\n")
	out.Write(body)
	return out.Bytes(), nil
}

func readRTSPResponse(conn net.Conn, maxBytes int) (int, string, map[string]string, []byte, error) {
	buf := []byte{}
	headerEnd := -1
	for headerEnd < 0 {
		if pos := bytes.Index(buf, []byte("\r\n\r\n")); pos >= 0 {
			headerEnd = pos
			break
		}
		if len(buf) > maxBytes {
			return 0, "", nil, nil, fmt.Errorf("rtsp response headers exceed max_bytes (%d)", maxBytes)
		}
		chunk := make([]byte, 4096)
		n, err := conn.Read(chunk)
		if n > 0 {
			buf = append(buf, chunk[:n]...)
		}
		if err != nil {
			if mediaTimeout(err) {
				return 0, "", nil, nil, err
			}
			if errors.Is(err, io.EOF) {
				return 0, "", nil, nil, fmt.Errorf("rtsp connection closed before response headers")
			}
			return 0, "", nil, nil, fmt.Errorf("rtsp read failed: %v", err)
		}
		if n == 0 {
			return 0, "", nil, nil, fmt.Errorf("rtsp connection closed before response headers")
		}
	}
	head := string(buf[:headerEnd])
	lines := strings.Split(head, "\r\n")
	parts := strings.SplitN(lines[0], " ", 3)
	if len(parts) < 2 || !strings.HasPrefix(parts[0], "RTSP/") {
		return 0, "", nil, nil, fmt.Errorf("invalid rtsp status line `%s`", lines[0])
	}
	status, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, "", nil, nil, fmt.Errorf("invalid rtsp status line `%s`", lines[0])
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
	length := uint64(0)
	if value, ok := mediaHeader(headers, "content-length"); ok {
		length, _ = strconv.ParseUint(strings.TrimSpace(value), 10, 64)
	}
	if length > uint64(maxBytes) {
		return 0, "", nil, nil, fmt.Errorf("rtsp response body exceeds max_bytes (%d)", maxBytes)
	}
	wanted := int(length)
	body := append([]byte{}, buf[headerEnd+4:]...)
	for len(body) < wanted {
		chunk := make([]byte, 4096)
		n, err := conn.Read(chunk)
		if n > 0 {
			body = append(body, chunk[:n]...)
		}
		if err != nil {
			if mediaTimeout(err) {
				return 0, "", nil, nil, err
			}
			return 0, "", nil, nil, fmt.Errorf("rtsp connection closed before Content-Length (%d) was read", wanted)
		}
		if n == 0 {
			return 0, "", nil, nil, fmt.Errorf("rtsp connection closed before Content-Length (%d) was read", wanted)
		}
	}
	return status, reason, headers, body[:wanted], nil
}

func rtspAuthError(status string, headers map[string]string, headerName string) error {
	scheme := "Digest/Basic"
	if value, ok := mediaHeader(headers, headerName); ok {
		if fields := strings.Fields(value); len(fields) > 0 {
			scheme = fields[0]
		}
	}
	return fmt.Errorf("RTSP %s: %s authentication is required but credentials-based auth is not supported by the debug executor", status, scheme)
}

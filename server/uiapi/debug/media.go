//go:build apidoc_debug_media || apidoc_debug_all

package debug

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"
)

// mediaMaxDatagrams is the per-call packet budget (plan max_frames).
const mediaMaxDatagrams = 1000

// mediaMaxDatagramBytes is the IPv4 UDP payload ceiling.
const mediaMaxDatagramBytes = 65507

func mediaRandomBytes(n int) []byte {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return make([]byte, n) // a zero nonce still round-trips; never fail on entropy
	}
	return buf
}

func mediaRandomHex(n int) string {
	return hex.EncodeToString(mediaRandomBytes(n))
}

// mediaWindow clamps opts.duration_ms to the request timeout.
func mediaWindow(cfg Config, req *Request, defaultMS uint64) time.Duration {
	ms := protocolOptUint(req, "duration_ms", defaultMS)
	limit := uint64(req.Timeout(cfg) / time.Millisecond)
	if limit < 1 {
		limit = 1
	}
	if ms < 1 {
		ms = 1
	}
	if ms > limit {
		ms = limit
	}
	return time.Duration(ms) * time.Millisecond
}

// mediaTimeout reports whether err is a deadline expiry.
func mediaTimeout(err error) bool {
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}

// mediaHeader looks up a header without depending on the server's casing.
func mediaHeader(headers map[string]string, name string) (string, bool) {
	for key, value := range headers {
		if strings.EqualFold(key, name) {
			return value, true
		}
	}
	return "", false
}

// mediaPayload renders a binary body in the UIC payload shape.
func mediaPayload(data []byte) map[string]string {
	payload := FromBytes(data)
	return map[string]string{"encoding": payload.Encoding, "data": payload.Data}
}

// mediaJSONResponse builds the unary response for packet-level executors.
func mediaJSONResponse(started time.Time, sent, received int, body any) (*Response, error) {
	encoded, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("encode response: %v", err)
	}
	return &Response{
		Status: 0,
		MS:     time.Since(started).Milliseconds(),
		Headers: map[string]string{
			"x-sent":     strconv.Itoa(sent),
			"x-received": strconv.Itoa(received),
		},
		Body: FromBytes(encoded),
	}, nil
}

// mediaUDPExchange sends count (1-1000) datagrams and, when opts.recv is
// "true", collects up to count replies within duration_ms.
func mediaUDPExchange(cfg Config, req *Request, conn net.Conn, build func(int) ([]byte, error)) (int, [][]byte, error) {
	count := int(protocolOptUint(req, "count", 1))
	if count < 1 {
		count = 1
	}
	if count > mediaMaxDatagrams {
		count = mediaMaxDatagrams
	}
	sentBytes := 0
	for i := 0; i < count; i++ {
		packet, err := build(i)
		if err != nil {
			return 0, nil, err
		}
		if len(packet) > mediaMaxDatagramBytes {
			return 0, nil, fmt.Errorf("udp packet exceeds %d bytes", mediaMaxDatagramBytes)
		}
		sentBytes += len(packet)
		if sentBytes > cfg.Limit() {
			return 0, nil, fmt.Errorf("udp send exceeds max_bytes (%d)", cfg.Limit())
		}
		if _, err := conn.Write(packet); err != nil {
			return 0, nil, fmt.Errorf("udp send failed: %v", err)
		}
	}
	packets := [][]byte{}
	if req.Opts["recv"] != "true" {
		return count, packets, nil
	}
	deadline := time.Now().Add(mediaWindow(cfg, req, 500))
	total := 0
	for i := 0; i < count; i++ {
		if err := conn.SetReadDeadline(deadline); err != nil {
			return 0, nil, fmt.Errorf("udp deadline failed: %v", err)
		}
		buf := make([]byte, mediaMaxDatagramBytes)
		n, err := conn.Read(buf)
		if err != nil {
			if mediaTimeout(err) {
				break
			}
			return 0, nil, fmt.Errorf("udp recv failed: %v", err)
		}
		total += n
		if total > cfg.Limit() {
			return 0, nil, fmt.Errorf("response exceeds max_bytes (%d)", cfg.Limit())
		}
		packets = append(packets, append([]byte{}, buf[:n]...))
	}
	return count, packets, nil
}

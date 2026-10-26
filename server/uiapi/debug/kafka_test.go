//go:build apidoc_debug_kafka || apidoc_debug_messaging || apidoc_debug_all

package debug

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"io"
	"net"
	"strings"
	"sync"
	"testing"
	"time"
)

type kafkaTestServer struct {
	addr   string
	topic  string
	leader string
	port   int
	values [][]byte

	authError string

	mu         sync.Mutex
	sasl       []byte
	fetchCount int64
}

func newKafkaTestServer(t *testing.T, values [][]byte) *kafkaTestServer {
	t.Helper()
	ln := listenLocal(t)
	port := ln.Addr().(*net.TCPAddr).Port
	srv := &kafkaTestServer{addr: ln.Addr().String(), topic: "events", leader: "127.0.0.1", port: port, values: values}
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go srv.handle(conn)
		}
	}()
	return srv
}

func kafkaTestInt16(value int16) []byte {
	out := make([]byte, 2)
	binary.BigEndian.PutUint16(out, uint16(value))
	return out
}

func kafkaTestInt32(value int32) []byte {
	out := make([]byte, 4)
	binary.BigEndian.PutUint32(out, uint32(value))
	return out
}

func kafkaTestInt64(value int64) []byte {
	out := make([]byte, 8)
	binary.BigEndian.PutUint64(out, uint64(value))
	return out
}

func kafkaTestVarint(value int64) []byte {
	encoded := uint64(value<<1) ^ uint64(value>>63)
	out := []byte{}
	for {
		digit := byte(encoded & 0x7f)
		encoded >>= 7
		if encoded != 0 {
			digit |= 0x80
		}
		out = append(out, digit)
		if encoded == 0 {
			return out
		}
	}
}

func kafkaTestRecords(values [][]byte) []byte {
	var records []byte
	for i, value := range values {
		body := []byte{0} // attributes
		body = append(body, kafkaTestVarint(0)...)
		body = append(body, kafkaTestVarint(int64(i))...)
		body = append(body, kafkaTestVarint(-1)...) // null key
		body = append(body, kafkaTestVarint(int64(len(value)))...)
		body = append(body, value...)
		body = append(body, 0) // no headers
		records = append(records, kafkaTestVarint(int64(len(body)))...)
		records = append(records, body...)
	}
	return records
}

func kafkaTestBatchEncoded(baseOffset int64, attributes int16, payload []byte, count int) []byte {
	batch := make([]byte, 49)
	batch[4] = 2 // magic
	binary.BigEndian.PutUint16(batch[9:], uint16(attributes))
	if count > 0 {
		binary.BigEndian.PutUint32(batch[11:], uint32(count-1)) // last_offset_delta
	}
	binary.BigEndian.PutUint32(batch[45:], uint32(count))
	batch = append(batch, payload...)
	out := make([]byte, 12)
	binary.BigEndian.PutUint64(out, uint64(baseOffset))
	binary.BigEndian.PutUint32(out[8:], uint32(len(batch)))
	return append(out, batch...)
}

func kafkaTestBatch(baseOffset int64, values [][]byte) []byte {
	return kafkaTestBatchEncoded(baseOffset, 0, kafkaTestRecords(values), len(values))
}

func kafkaTestResponse(conn net.Conn, corr int32, body []byte) error {
	frame := make([]byte, 8, 8+len(body))
	binary.BigEndian.PutUint32(frame, uint32(4+len(body)))
	binary.BigEndian.PutUint32(frame[4:], uint32(corr))
	frame = append(frame, body...)
	_, err := conn.Write(frame)
	return err
}

func (s *kafkaTestServer) metadata() []byte {
	body := kafkaTestInt32(0)
	body = append(body, kafkaTestInt32(1)...) // brokers
	body = append(body, kafkaTestInt32(1)...) // node id
	body = append(body, kafkaString(s.leader)...)
	body = append(body, kafkaTestInt32(int32(s.port))...)
	body = append(body, 0xff, 0xff)           // rack
	body = append(body, 0xff, 0xff)           // cluster id
	body = append(body, kafkaTestInt32(1)...) // controller
	body = append(body, kafkaTestInt32(1)...) // topics
	body = append(body, kafkaTestInt16(0)...)
	body = append(body, kafkaString(s.topic)...)
	body = append(body, 0)                    // is_internal
	body = append(body, kafkaTestInt32(1)...) // partitions
	body = append(body, kafkaTestInt16(0)...) // partition error
	body = append(body, kafkaTestInt32(0)...) // partition index
	body = append(body, kafkaTestInt32(1)...) // leader
	body = append(body, kafkaTestInt32(1)...) // replicas count
	body = append(body, kafkaTestInt32(1)...) // replica 1
	body = append(body, kafkaTestInt32(1)...) // isr count
	body = append(body, kafkaTestInt32(1)...) // isr 1
	return body
}

func (s *kafkaTestServer) listOffsets(topic string) []byte {
	body := kafkaTestInt32(1) // topics
	body = append(body, kafkaString(topic)...)
	body = append(body, kafkaTestInt32(1)...) // partitions
	body = append(body, kafkaTestInt32(0)...)
	body = append(body, kafkaTestInt16(0)...)
	body = append(body, kafkaTestInt64(-1)...) // timestamp
	body = append(body, kafkaTestInt64(0)...)  // offset
	return body
}

func (s *kafkaTestServer) fetch(topic string) []byte {
	batch := []byte{}
	if len(s.values) > 0 {
		batch = kafkaTestBatch(0, s.values)
	}
	body := kafkaTestInt32(0) // throttle
	body = append(body, kafkaTestInt32(1)...)
	body = append(body, kafkaString(topic)...)
	body = append(body, kafkaTestInt32(1)...) // partitions
	body = append(body, kafkaTestInt32(0)...)
	body = append(body, kafkaTestInt16(0)...)
	body = append(body, kafkaTestInt64(int64(len(s.values)))...) // high watermark
	body = append(body, kafkaTestInt64(int64(len(s.values)))...) // last stable offset
	body = append(body, kafkaTestInt32(-1)...)                   // aborted transactions
	body = append(body, kafkaBytes(batch)...)
	return body
}

func (s *kafkaTestServer) handle(conn net.Conn) {
	defer conn.Close()
	for {
		var head [4]byte
		if _, err := io.ReadFull(conn, head[:]); err != nil {
			return
		}
		size := int(binary.BigEndian.Uint32(head[:]))
		if size < 10 || size > 1<<20 {
			return
		}
		payload := make([]byte, size)
		if _, err := io.ReadFull(conn, payload); err != nil {
			return
		}
		api := int16(binary.BigEndian.Uint16(payload))
		corr := int32(binary.BigEndian.Uint32(payload[4:]))
		clientID := int(binary.BigEndian.Uint16(payload[8:]))
		body := payload[10+clientID:]

		var response []byte
		switch api {
		case 17: // SaslHandshake
			response = kafkaTestInt16(0)
			response = append(response, kafkaTestInt32(1)...)
			response = append(response, kafkaString("PLAIN")...)
		case 36: // SaslAuthenticate
			if len(body) >= 4 {
				length := int(int32(binary.BigEndian.Uint32(body)))
				if length >= 0 && 4+length <= len(body) {
					s.mu.Lock()
					if s.sasl == nil {
						s.sasl = append([]byte{}, body[4:4+length]...)
					}
					s.mu.Unlock()
				}
			}
			if s.authError != "" {
				response = kafkaTestInt16(58)
				response = append(response, kafkaString(s.authError)...)
			} else {
				response = kafkaTestInt16(0)
				response = append(response, 0xff, 0xff) // null error message
				response = append(response, kafkaTestInt32(0)...)
			}
		case 3: // Metadata
			response = s.metadata()
		case 2: // ListOffsets
			response = s.listOffsets(s.topic)
		case 1: // Fetch
			s.mu.Lock()
			s.fetchCount++
			s.mu.Unlock()
			response = s.fetch(s.topic)
		default:
			return
		}
		if kafkaTestResponse(conn, corr, response) != nil {
			return
		}
	}
}

func kafkaTestRequest(addr string, opts map[string]string) *Request {
	all := map[string]string{}
	for key, value := range opts {
		all[key] = value
	}
	return &Request{Kind: "kafka", Target: "kafka://" + addr, Opts: all}
}

func TestKafkaConsume(t *testing.T) {
	srv := newKafkaTestServer(t, [][]byte{[]byte("one"), []byte("two")})
	resp, err := Execute(messagingConfig(), kafkaTestRequest(srv.addr, map[string]string{
		"topic": "events", "messages": "2", "duration_ms": "500",
	}))
	if err != nil {
		t.Fatal(err)
	}
	body := decodeMessagingJSON(t, resp)
	messages := body["messages"].([]any)
	if len(messages) != 2 {
		t.Fatalf("messages = %#v", messages)
	}
	for i, expected := range []string{"one", "two"} {
		message := messages[i].(map[string]any)
		if message["partition"] != float64(0) || message["offset"] != float64(i) {
			t.Fatalf("message %d = %#v", i, message)
		}
		if messagingPayload(message)["data"] != expected {
			t.Fatalf("message %d payload = %#v", i, messagingPayload(message))
		}
	}
	if resp.Headers["x-topic"] != "events" || resp.Headers["x-messages"] != "2" {
		t.Fatalf("headers = %#v", resp.Headers)
	}
}

func TestKafkaWindowAndValidation(t *testing.T) {
	empty := newKafkaTestServer(t, nil)
	started := time.Now()
	resp, err := Execute(messagingConfig(), kafkaTestRequest(empty.addr, map[string]string{
		"topic": "events", "duration_ms": "40",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("empty window took %v", elapsed)
	}
	if messages := decodeMessagingJSON(t, resp)["messages"].([]any); len(messages) != 0 {
		t.Fatalf("messages = %#v", messages)
	}

	for name, req := range map[string]*Request{
		"missing topic":     {Kind: "kafka", Target: "kafka://127.0.0.1:1"},
		"invalid partition": {Kind: "kafka", Target: "kafka://127.0.0.1:1", Opts: map[string]string{"topic": "events", "partition": "-1"}},
		"invalid offset":    {Kind: "kafka", Target: "kafka://127.0.0.1:1", Opts: map[string]string{"topic": "events", "offset": "middle"}},
		"tls":               {Kind: "kafka", Target: "kafkas://127.0.0.1:1", Opts: map[string]string{"topic": "events"}},
		"password only":     {Kind: "kafka", Target: "kafka://127.0.0.1:1", Opts: map[string]string{"topic": "events", "password": "pw"}},
	} {
		if _, err := Execute(messagingConfig(), req); err == nil {
			t.Fatalf("%s was accepted", name)
		}
	}
}

func TestKafkaCapsSSRFAndMetadataBroker(t *testing.T) {
	big := newKafkaTestServer(t, [][]byte{[]byte(strings.Repeat("x", 100))})
	cfg := messagingConfig()
	cfg.MaxBytes = 64
	_, err := Execute(cfg, kafkaTestRequest(big.addr, map[string]string{"topic": "events"}))
	if err == nil || !strings.Contains(err.Error(), "max_bytes") {
		t.Fatalf("cap err = %v", err)
	}

	private := Config{EnableServerSide: true, TimeoutMS: 500}
	_, err = Execute(private, kafkaTestRequest(big.addr, map[string]string{"topic": "events"}))
	if err == nil || !strings.Contains(err.Error(), "private") {
		t.Fatalf("ssrf err = %v", err)
	}

	// A metadata-advertised broker is always checked by the guard, even when
	// private networks are allowed: cloud metadata addresses stay blocked.
	trap := newKafkaTestServer(t, nil)
	trap.leader = "169.254.169.254"
	_, err = Execute(messagingConfig(), kafkaTestRequest(trap.addr, map[string]string{"topic": "events"}))
	if err == nil || !strings.Contains(err.Error(), "metadata broker") || !strings.Contains(err.Error(), "blocked") {
		t.Fatalf("metadata ssrf err = %v", err)
	}
}

func TestKafkaSASL(t *testing.T) {
	srv := newKafkaTestServer(t, [][]byte{[]byte("one")})
	resp, err := Execute(messagingConfig(), kafkaTestRequest(srv.addr, map[string]string{
		"topic": "events", "username": "user", "password": "pw-secret",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if messages := decodeMessagingJSON(t, resp)["messages"].([]any); len(messages) != 1 {
		t.Fatalf("messages = %#v", messages)
	}
	srv.mu.Lock()
	sasl := string(srv.sasl)
	srv.mu.Unlock()
	if sasl != "\x00user\x00pw-secret" {
		t.Fatalf("sasl = %q", sasl)
	}

	failing := newKafkaTestServer(t, nil)
	failing.authError = "rejected password pw-secret"
	_, err = Execute(messagingConfig(), kafkaTestRequest(failing.addr, map[string]string{
		"topic": "events", "username": "user", "password": "pw-secret",
	}))
	if err == nil || !strings.Contains(err.Error(), "SASL authentication failed") {
		t.Fatalf("auth err = %v", err)
	}
	if strings.Contains(err.Error(), "pw-secret") || !strings.Contains(err.Error(), "***") {
		t.Fatalf("auth error leaked the password: %v", err)
	}
}

func TestKafkaStreamSession(t *testing.T) {
	srv := newKafkaTestServer(t, [][]byte{[]byte("one"), []byte("two")})
	stream, err := OpenStream(messagingConfig(), kafkaTestRequest(srv.addr, map[string]string{"topic": "events"}))
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	frame, err := stream.ReadFrame()
	if err != nil || string(frame) != "one" {
		t.Fatalf("first = %q err=%v", frame, err)
	}
	frame, err = stream.ReadFrame()
	if err != nil || string(frame) != "two" {
		t.Fatalf("second = %q err=%v", frame, err)
	}
	if err := stream.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestKafkaParseRecords(t *testing.T) {
	values, offsets, err := parseKafkaRecords(kafkaTestBatch(10, [][]byte{[]byte("a"), []byte("b")}), 1<<20)
	if err != nil || len(values) != 2 || string(values[0]) != "a" || offsets[0] != 10 || offsets[1] != 11 {
		t.Fatalf("records = %q %v err=%v", values, offsets, err)
	}

	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	if _, err := writer.Write(kafkaTestRecords([][]byte{[]byte("gz")})); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	values, offsets, err = parseKafkaRecords(kafkaTestBatchEncoded(5, 1, compressed.Bytes(), 1), 1<<20)
	if err != nil || len(values) != 1 || string(values[0]) != "gz" || offsets[0] != 5 {
		t.Fatalf("gzip records = %q %v err=%v", values, offsets, err)
	}

	if _, _, err := parseKafkaRecords(kafkaTestBatchEncoded(0, 2, []byte("x"), 1), 1<<20); err == nil || !strings.Contains(err.Error(), "compression codec") {
		t.Fatalf("codec err = %v", err)
	}
	badMagic := kafkaTestBatch(0, [][]byte{[]byte("a")})
	badMagic[16] = 1 // magic lives at offset 16 (base 8 + length 4 + leader epoch 4)
	if _, _, err := parseKafkaRecords(badMagic, 1<<20); err == nil || !strings.Contains(err.Error(), "magic") {
		t.Fatalf("magic err = %v", err)
	}
	if _, _, err := parseKafkaRecords(kafkaTestBatch(0, [][]byte{bytes.Repeat([]byte("a"), 100)})[:70], 1<<20); err == nil {
		t.Fatal("truncated batch was accepted")
	}
}

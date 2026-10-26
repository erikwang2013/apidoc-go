//go:build apidoc_debug_kafka || apidoc_debug_messaging || apidoc_debug_all

package debug

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"time"
)

func init() {
	RegisterExecutor("kafka", executeKafka)
	RegisterStream("kafka", openKafkaStream)
}

// kafkaConn speaks the subset of the Kafka protocol the relay needs:
// Metadata v4, ListOffsets v1, Fetch v4 (record batch magic 2) and SASL PLAIN.
// It never writes to the cluster.
type kafkaConn struct {
	conn     net.Conn
	limit    int
	password string
	corr     int32
}

func kafkaString(value string) []byte {
	out := make([]byte, 2, 2+len(value))
	binary.BigEndian.PutUint16(out, uint16(len(value)))
	return append(out, value...)
}

func kafkaBytes(value []byte) []byte {
	out := make([]byte, 4, 4+len(value))
	binary.BigEndian.PutUint32(out, uint32(len(value)))
	return append(out, value...)
}

func (k *kafkaConn) request(apiKey, version int16, body []byte) ([]byte, error) {
	k.corr++
	payload := make([]byte, 10, 10+len(body))
	binary.BigEndian.PutUint16(payload, uint16(apiKey))
	binary.BigEndian.PutUint16(payload[2:], uint16(version))
	binary.BigEndian.PutUint32(payload[4:], uint32(k.corr))
	binary.BigEndian.PutUint16(payload[8:], uint16(len("apidoc")))
	payload = append(payload, "apidoc"...)
	payload = append(payload, body...)
	frame := make([]byte, 4, 4+len(payload))
	binary.BigEndian.PutUint32(frame, uint32(len(payload)))
	frame = append(frame, payload...)
	if _, err := k.conn.Write(frame); err != nil {
		return nil, err
	}
	var lengthBytes [4]byte
	if _, err := io.ReadFull(k.conn, lengthBytes[:]); err != nil {
		return nil, err
	}
	length := int(int32(binary.BigEndian.Uint32(lengthBytes[:])))
	if length < 4 || length > k.limit {
		return nil, fmt.Errorf("kafka response length %d exceeds max_bytes", length)
	}
	response := make([]byte, length)
	if _, err := io.ReadFull(k.conn, response); err != nil {
		return nil, err
	}
	if int32(binary.BigEndian.Uint32(response)) != k.corr {
		return nil, fmt.Errorf("kafka correlation id mismatch")
	}
	return response[4:], nil
}

func kafkaConnect(cfg Config, broker kafkaBroker, timeout time.Duration, username, password string) (*kafkaConn, error) {
	conn, err := dialMessaging(cfg, broker.host, broker.port, timeout)
	if err != nil {
		return nil, fmt.Errorf("kafka %v", err)
	}
	client := &kafkaConn{conn: conn, limit: cfg.Limit(), password: password}
	if err := conn.SetDeadline(time.Now().Add(timeout)); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("kafka deadline failed: %v", err)
	}
	if username != "" {
		if err := client.saslPlain(username, password); err != nil {
			_ = conn.Close()
			return nil, client.redact(err.Error())
		}
	}
	_ = conn.SetDeadline(time.Time{})
	return client, nil
}

func (k *kafkaConn) redact(text string) error {
	return fmt.Errorf("%s", redactSecrets(text, k.password))
}

func (k *kafkaConn) saslPlain(username, password string) error {
	response, err := k.request(17, 1, kafkaString("PLAIN")) // SaslHandshake
	if err != nil {
		return fmt.Errorf("kafka SASL handshake failed: %v", err)
	}
	reader := kafkaReader{data: response}
	code, err := reader.int16()
	if err != nil {
		return fmt.Errorf("kafka SASL handshake failed: %v", err)
	}
	if code != 0 {
		return fmt.Errorf("kafka SASL handshake failed: %s", kafkaErrorText(code))
	}
	count, err := reader.arrayLen(64)
	if err != nil {
		return fmt.Errorf("invalid kafka SASL mechanism list: %v", err)
	}
	offered := false
	for i := 0; i < count; i++ {
		mechanism, err := reader.string()
		if err != nil {
			return fmt.Errorf("truncated kafka SASL mechanism list")
		}
		if mechanism == "PLAIN" {
			offered = true
		}
	}
	if !offered {
		return fmt.Errorf("kafka broker does not offer SASL PLAIN")
	}
	auth := []byte("\x00" + username + "\x00" + password)
	response, err = k.request(36, 0, kafkaBytes(auth)) // SaslAuthenticate
	if err != nil {
		return fmt.Errorf("kafka SASL authentication failed: %v", err)
	}
	reader = kafkaReader{data: response}
	code, err = reader.int16()
	if err != nil {
		return fmt.Errorf("kafka SASL authentication failed: %v", err)
	}
	message, err := reader.nullableString()
	if err != nil {
		return fmt.Errorf("kafka SASL authentication failed: %v", err)
	}
	if code != 0 {
		if message == "" {
			message = kafkaErrorText(code)
		}
		return fmt.Errorf("kafka SASL authentication failed: %s", message)
	}
	return nil
}

type kafkaReader struct {
	data []byte
	pos  int
}

func (r *kafkaReader) take(n int) ([]byte, error) {
	if n < 0 || r.pos+n > len(r.data) {
		return nil, fmt.Errorf("truncated kafka response")
	}
	value := r.data[r.pos : r.pos+n]
	r.pos += n
	return value, nil
}

func (r *kafkaReader) int8() (int8, error) {
	b, err := r.take(1)
	if err != nil {
		return 0, err
	}
	return int8(b[0]), nil
}

func (r *kafkaReader) int16() (int16, error) {
	b, err := r.take(2)
	if err != nil {
		return 0, err
	}
	return int16(binary.BigEndian.Uint16(b)), nil
}

func (r *kafkaReader) int32() (int32, error) {
	b, err := r.take(4)
	if err != nil {
		return 0, err
	}
	return int32(binary.BigEndian.Uint32(b)), nil
}

func (r *kafkaReader) int64() (int64, error) {
	b, err := r.take(8)
	if err != nil {
		return 0, err
	}
	return int64(binary.BigEndian.Uint64(b)), nil
}

func (r *kafkaReader) string() (string, error) {
	length, err := r.int16()
	if err != nil || length < 0 {
		return "", fmt.Errorf("truncated kafka response")
	}
	b, err := r.take(int(length))
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func (r *kafkaReader) nullableString() (string, error) {
	length, err := r.int16()
	if err != nil {
		return "", err
	}
	if length < 0 {
		return "", nil
	}
	b, err := r.take(int(length))
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func (r *kafkaReader) bytes() ([]byte, error) {
	length, err := r.int32()
	if err != nil {
		return nil, err
	}
	if length < 0 {
		return nil, nil
	}
	return r.take(int(length))
}

func (r *kafkaReader) arrayLen(max int) (int, error) {
	count, err := r.int32()
	if err != nil {
		return 0, err
	}
	if count < -1 || count > int32(max) {
		return 0, fmt.Errorf("invalid kafka array length %d", count)
	}
	if count < 0 {
		return 0, nil
	}
	return int(count), nil
}

func kafkaErrorText(code int16) string {
	switch code {
	case 1:
		return "offset out of range"
	case 2:
		return "invalid message"
	case 3:
		return "unknown topic or partition"
	case 5:
		return "leader not available"
	case 6:
		return "not leader for partition"
	case 7:
		return "request timed out"
	case 29:
		return "topic authorization failed"
	case 30:
		return "group authorization failed"
	case 31:
		return "cluster authorization failed"
	case 33:
		return "unsupported SASL mechanism"
	case 34:
		return "illegal SASL state"
	case 35:
		return "unsupported version"
	case 58:
		return "SASL authentication failed"
	default:
		return fmt.Sprintf("error %d", code)
	}
}

type kafkaBroker struct {
	nodeID int32
	host   string
	port   string
}

func kafkaTopic(req *Request) (string, error) {
	topic := req.Opts["topic"]
	if topic == "" {
		return "", fmt.Errorf("kafka requires opts.topic")
	}
	if len(topic) > 249 {
		return "", fmt.Errorf("kafka topic is too long")
	}
	return topic, nil
}

func kafkaPartition(req *Request) (int32, error) {
	raw := req.Opts["partition"]
	if raw == "" {
		return 0, nil
	}
	value, err := strconv.ParseInt(raw, 10, 32)
	if err != nil || value < 0 {
		return 0, fmt.Errorf("invalid kafka partition `%s`; use a non-negative integer", raw)
	}
	return int32(value), nil
}

// kafkaBrokers resolves each bootstrap broker and checks it against the guard.
func kafkaBrokers(cfg Config, target string) ([]kafkaBroker, error) {
	list := target
	if strings.HasPrefix(list, "kafkas://") {
		return nil, fmt.Errorf("kafka TLS (kafkas://) is not supported by this build; use kafka:// (plaintext)")
	}
	if strings.Contains(list, "://") {
		if !strings.HasPrefix(list, "kafka://") {
			return nil, fmt.Errorf("unsupported kafka scheme; use kafka:// (plaintext only)")
		}
		list = strings.TrimPrefix(list, "kafka://")
	}
	brokers := []kafkaBroker{}
	for _, entry := range strings.Split(list, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			return nil, fmt.Errorf("kafka target contains an empty broker address")
		}
		host, port, _, err := parseMessagingTarget(entry, "kafka", "9092", map[string]bool{"kafka": true})
		if err != nil {
			return nil, err
		}
		if _, err := CheckHost(cfg, host, port); err != nil {
			return nil, err
		}
		brokers = append(brokers, kafkaBroker{host: host, port: port})
	}
	return brokers, nil
}

// kafkaMetadata fetches broker metadata and returns the node id of the leader
// for the requested partition. Every advertised broker is checked by the guard
// before the leader connection is attempted (same residual TOCTOU as Rust).
func kafkaMetadata(client *kafkaConn, topic string, partition int32, cfg Config) ([]kafkaBroker, int32, error) {
	request := make([]byte, 4)
	binary.BigEndian.PutUint32(request, 1)
	request = append(request, kafkaString(topic)...)
	request = append(request, 0) // allow_auto_topic_creation=false
	response, err := client.request(3, 4, request)
	if err != nil {
		return nil, 0, fmt.Errorf("kafka metadata failed: %v", err)
	}
	reader := kafkaReader{data: response}
	if _, err := reader.int32(); err != nil { // throttle_time_ms
		return nil, 0, err
	}
	brokerCount, err := reader.arrayLen(1000)
	if err != nil {
		return nil, 0, err
	}
	brokers := []kafkaBroker{}
	for i := 0; i < brokerCount; i++ {
		nodeID, err := reader.int32()
		if err != nil {
			return nil, 0, err
		}
		host, err := reader.string()
		if err != nil {
			return nil, 0, err
		}
		port, err := reader.int32()
		if err != nil {
			return nil, 0, err
		}
		if _, err := reader.nullableString(); err != nil { // rack
			return nil, 0, err
		}
		if host == "" || port < 1 || port > 65535 {
			return nil, 0, fmt.Errorf("invalid kafka broker address `%s:%d`", host, port)
		}
		if _, err := CheckHost(cfg, host, strconv.Itoa(int(port))); err != nil {
			return nil, 0, fmt.Errorf("kafka metadata broker `%s:%d` rejected: %v", host, port, err)
		}
		brokers = append(brokers, kafkaBroker{nodeID: nodeID, host: host, port: strconv.Itoa(int(port))})
	}
	if len(brokers) == 0 {
		return nil, 0, fmt.Errorf("kafka metadata returned no brokers")
	}
	if _, err := reader.nullableString(); err != nil { // cluster_id
		return nil, 0, err
	}
	if _, err := reader.int32(); err != nil { // controller_id
		return nil, 0, err
	}
	topicCount, err := reader.arrayLen(10000)
	if err != nil {
		return nil, 0, err
	}
	for i := 0; i < topicCount; i++ {
		errorCode, err := reader.int16()
		if err != nil {
			return nil, 0, err
		}
		name, err := reader.string()
		if err != nil {
			return nil, 0, err
		}
		if _, err := reader.int8(); err != nil { // is_internal
			return nil, 0, err
		}
		partitionCount, err := reader.arrayLen(100000)
		if err != nil {
			return nil, 0, err
		}
		if name != topic {
			for p := 0; p < partitionCount; p++ {
				if err := reader.skipPartition(); err != nil {
					return nil, 0, err
				}
			}
			continue
		}
		if errorCode != 0 {
			return nil, 0, fmt.Errorf("kafka topic `%s` unavailable: %s", topic, kafkaErrorText(errorCode))
		}
		for p := 0; p < partitionCount; p++ {
			partitionError, err := reader.int16()
			if err != nil {
				return nil, 0, err
			}
			index, err := reader.int32()
			if err != nil {
				return nil, 0, err
			}
			leader, err := reader.int32()
			if err != nil {
				return nil, 0, err
			}
			if err := reader.skipInt32Array(1000); err != nil { // replicas
				return nil, 0, err
			}
			if err := reader.skipInt32Array(1000); err != nil { // isr
				return nil, 0, err
			}
			if index == partition {
				if partitionError != 0 {
					return nil, 0, fmt.Errorf("kafka partition %d unavailable: %s", partition, kafkaErrorText(partitionError))
				}
				return brokers, leader, nil
			}
		}
		return nil, 0, fmt.Errorf("kafka topic `%s` has no partition %d", topic, partition)
	}
	return nil, 0, fmt.Errorf("kafka metadata did not return topic `%s`", topic)
}

func (r *kafkaReader) skipPartition() error {
	if _, err := r.int16(); err != nil { // error_code
		return err
	}
	if _, err := r.int32(); err != nil { // partition_index
		return err
	}
	if _, err := r.int32(); err != nil { // leader_id
		return err
	}
	if err := r.skipInt32Array(1000); err != nil { // replicas
		return err
	}
	return r.skipInt32Array(1000) // isr
}

func (r *kafkaReader) skipInt32Array(max int) error {
	count, err := r.arrayLen(max)
	if err != nil {
		return err
	}
	_, err = r.take(4 * count)
	return err
}

func kafkaListOffset(client *kafkaConn, topic string, partition int32, at string) (int64, error) {
	timestamp := int64(-1) // latest
	if at == "earliest" {
		timestamp = -2
	}
	body := make([]byte, 4)
	binary.BigEndian.PutUint32(body, 0xffffffff) // replica id -1
	body = append(body, 0, 0, 0, 1)
	body = append(body, kafkaString(topic)...)
	body = append(body, 0, 0, 0, 1)
	body = append(body, make([]byte, 4)...)
	binary.BigEndian.PutUint32(body[len(body)-4:], uint32(partition))
	body = append(body, make([]byte, 8)...)
	binary.BigEndian.PutUint64(body[len(body)-8:], uint64(timestamp))
	response, err := client.request(2, 1, body)
	if err != nil {
		return 0, fmt.Errorf("kafka offset lookup failed: %v", err)
	}
	reader := kafkaReader{data: response}
	topics, err := reader.arrayLen(10000)
	if err != nil {
		return 0, err
	}
	for i := 0; i < topics; i++ {
		name, err := reader.string()
		if err != nil {
			return 0, err
		}
		count, err := reader.arrayLen(100000)
		if err != nil {
			return 0, err
		}
		for p := 0; p < count; p++ {
			index, err := reader.int32()
			if err != nil {
				return 0, err
			}
			code, err := reader.int16()
			if err != nil {
				return 0, err
			}
			if _, err := reader.int64(); err != nil { // timestamp
				return 0, err
			}
			offset, err := reader.int64()
			if err != nil {
				return 0, err
			}
			if name == topic && index == partition {
				if code != 0 {
					return 0, fmt.Errorf("kafka offset lookup failed: %s", kafkaErrorText(code))
				}
				if offset < 0 {
					return 0, fmt.Errorf("kafka offset lookup returned a negative offset")
				}
				return offset, nil
			}
		}
	}
	return 0, fmt.Errorf("kafka offset lookup returned no partition")
}

func kafkaFetch(client *kafkaConn, topic string, partition int32, offset int64, limit int) ([][]byte, []int64, error) {
	if limit < 1 {
		limit = 1
	}
	body := make([]byte, 4)
	binary.BigEndian.PutUint32(body, 0xffffffff) // replica id -1
	body = append(body, make([]byte, 4)...)      // max_wait_ms
	binary.BigEndian.PutUint32(body[len(body)-4:], 500)
	body = append(body, 0, 0, 0, 1) // min_bytes
	body = append(body, make([]byte, 4)...)
	binary.BigEndian.PutUint32(body[len(body)-4:], uint32(limit))
	body = append(body, 0) // isolation_level=read_uncommitted
	body = append(body, 0, 0, 0, 1)
	body = append(body, kafkaString(topic)...)
	body = append(body, 0, 0, 0, 1)
	body = append(body, make([]byte, 4)...)
	binary.BigEndian.PutUint32(body[len(body)-4:], uint32(partition))
	body = append(body, make([]byte, 8)...)
	binary.BigEndian.PutUint64(body[len(body)-8:], uint64(offset))
	body = append(body, make([]byte, 4)...)
	binary.BigEndian.PutUint32(body[len(body)-4:], uint32(limit))
	response, err := client.request(1, 4, body)
	if err != nil {
		return nil, nil, fmt.Errorf("kafka fetch failed: %w", err)
	}
	reader := kafkaReader{data: response}
	if _, err := reader.int32(); err != nil { // throttle_time_ms
		return nil, nil, err
	}
	topics, err := reader.arrayLen(10000)
	if err != nil {
		return nil, nil, err
	}
	for i := 0; i < topics; i++ {
		name, err := reader.string()
		if err != nil {
			return nil, nil, err
		}
		count, err := reader.arrayLen(100000)
		if err != nil {
			return nil, nil, err
		}
		for p := 0; p < count; p++ {
			index, err := reader.int32()
			if err != nil {
				return nil, nil, err
			}
			code, err := reader.int16()
			if err != nil {
				return nil, nil, err
			}
			if _, err := reader.int64(); err != nil { // high_watermark
				return nil, nil, err
			}
			if _, err := reader.int64(); err != nil { // last_stable_offset
				return nil, nil, err
			}
			aborted, err := reader.arrayLen(10000)
			if err != nil {
				return nil, nil, err
			}
			if _, err := reader.take(16 * aborted); err != nil {
				return nil, nil, err
			}
			records, err := reader.bytes()
			if err != nil {
				return nil, nil, err
			}
			if name != topic || index != partition {
				continue
			}
			if code != 0 {
				return nil, nil, fmt.Errorf("kafka fetch failed: %s", kafkaErrorText(code))
			}
			return parseKafkaRecords(records, limit)
		}
	}
	return nil, nil, fmt.Errorf("kafka fetch returned no partition data")
}

// parseKafkaRecords decodes record batch magic 2. Only uncompressed and gzip
// batches are supported; other codecs fail explicitly instead of returning junk.
func parseKafkaRecords(data []byte, limit int) ([][]byte, []int64, error) {
	values := [][]byte{}
	offsets := []int64{}
	pos := 0
	for pos+61 <= len(data) {
		baseOffset := int64(binary.BigEndian.Uint64(data[pos:]))
		batchLength := int(int32(binary.BigEndian.Uint32(data[pos+8:])))
		if batchLength < 49 || pos+12+batchLength > len(data) {
			return nil, nil, fmt.Errorf("invalid kafka record batch length")
		}
		batch := data[pos+12 : pos+12+batchLength]
		if batch[4] != 2 {
			return nil, nil, fmt.Errorf("unsupported kafka record batch magic %d", batch[4])
		}
		attributes := int16(binary.BigEndian.Uint16(batch[9:]))
		recordCount := int32(binary.BigEndian.Uint32(batch[45:]))
		if recordCount < 0 || recordCount > 100000 {
			return nil, nil, fmt.Errorf("invalid kafka record count")
		}
		records := batch[49:]
		switch attributes & 0x07 {
		case 0:
		case 1:
			reader, err := gzip.NewReader(bytes.NewReader(records))
			if err != nil {
				return nil, nil, fmt.Errorf("invalid kafka gzip batch: %v", err)
			}
			uncompressed, err := io.ReadAll(io.LimitReader(reader, int64(limit)+1))
			_ = reader.Close()
			if err != nil {
				return nil, nil, fmt.Errorf("invalid kafka gzip batch: %v", err)
			}
			if len(uncompressed) > limit {
				return nil, nil, fmt.Errorf("kafka messages exceed max_bytes (%d)", limit)
			}
			records = uncompressed
		default:
			return nil, nil, fmt.Errorf("unsupported kafka compression codec %d", attributes&0x07)
		}
		record := kafkaReader{data: records}
		for i := int32(0); i < recordCount; i++ {
			value, offset, err := record.record(baseOffset)
			if err != nil {
				return nil, nil, err
			}
			values = append(values, value)
			offsets = append(offsets, offset)
		}
		pos += 12 + batchLength
	}
	return values, offsets, nil
}

// record decodes one record batch v2 record and returns its value and offset.
func (r *kafkaReader) record(baseOffset int64) ([]byte, int64, error) {
	length, err := r.varint()
	if err != nil {
		return nil, 0, err
	}
	end := r.pos + int(length)
	if length < 0 || end > len(r.data) {
		return nil, 0, fmt.Errorf("invalid kafka record length")
	}
	if _, err := r.take(1); err != nil { // attributes
		return nil, 0, err
	}
	if _, err := r.varint(); err != nil { // timestamp delta
		return nil, 0, err
	}
	offsetDelta, err := r.varint()
	if err != nil {
		return nil, 0, err
	}
	keyLength, err := r.varint()
	if err != nil {
		return nil, 0, err
	}
	if keyLength > 0 {
		if _, err := r.take(int(keyLength)); err != nil {
			return nil, 0, err
		}
	}
	valueLength, err := r.varint()
	if err != nil {
		return nil, 0, err
	}
	var value []byte
	if valueLength > 0 {
		if value, err = r.take(int(valueLength)); err != nil {
			return nil, 0, err
		}
	}
	headers, err := r.varint()
	if err != nil {
		return nil, 0, err
	}
	for i := int64(0); i < headers; i++ {
		for j := 0; j < 2; j++ { // header key then value
			size, err := r.varint()
			if err != nil {
				return nil, 0, err
			}
			if size > 0 {
				if _, err := r.take(int(size)); err != nil {
					return nil, 0, err
				}
			}
		}
	}
	if r.pos > end {
		return nil, 0, fmt.Errorf("kafka record is malformed")
	}
	r.pos = end
	return value, baseOffset + offsetDelta, nil
}

func (r *kafkaReader) varint() (int64, error) {
	var value uint64
	var shift uint
	for i := 0; i < 10; i++ {
		b, err := r.int8()
		if err != nil {
			return 0, err
		}
		value |= uint64(uint8(b)&0x7f) << shift
		if uint8(b)&0x80 == 0 {
			return int64(value>>1) ^ -int64(value&1), nil
		}
		shift += 7
	}
	return 0, fmt.Errorf("kafka varint is too long")
}

type kafkaClient struct {
	cfg       Config
	topic     string
	partition int32
	offset    int64
	leader    *kafkaConn
	password  string
}

func kafkaOpen(cfg Config, req *Request) (*kafkaClient, error) {
	topic, err := kafkaTopic(req)
	if err != nil {
		return nil, err
	}
	partition, err := kafkaPartition(req)
	if err != nil {
		return nil, err
	}
	username, password := req.Opts["username"], req.Opts["password"]
	if password != "" && username == "" {
		return nil, fmt.Errorf("kafka opts.password requires opts.username")
	}
	at := req.Opts["offset"]
	if at == "" {
		at = "latest"
	}
	if at != "latest" && at != "earliest" {
		if _, err := strconv.ParseInt(at, 10, 64); err != nil {
			return nil, fmt.Errorf("invalid kafka offset `%s`; use earliest, latest or a number", at)
		}
	}
	brokers, err := kafkaBrokers(cfg, req.Target)
	if err != nil {
		return nil, err
	}
	bootstrap, err := kafkaConnect(cfg, brokers[0], req.Timeout(cfg), username, password)
	if err != nil {
		return nil, err
	}
	metadataBrokers, leaderID, err := kafkaMetadata(bootstrap, topic, partition, cfg)
	_ = bootstrap.conn.Close()
	if err != nil {
		return nil, redactedError(err.Error(), password)
	}
	leader := kafkaBroker{}
	found := false
	for _, broker := range metadataBrokers {
		if broker.nodeID == leaderID {
			leader, found = broker, true
			break
		}
	}
	if !found {
		return nil, fmt.Errorf("kafka metadata leader %d is not in the broker list", leaderID)
	}
	conn, err := kafkaConnect(cfg, leader, req.Timeout(cfg), username, password)
	if err != nil {
		return nil, err
	}
	client := &kafkaClient{cfg: cfg, topic: topic, partition: partition, leader: conn, password: password}
	if at == "latest" || at == "earliest" {
		offset, err := kafkaListOffset(conn, topic, partition, at)
		if err != nil {
			_ = conn.conn.Close()
			return nil, conn.redact(err.Error())
		}
		client.offset = offset
	} else {
		client.offset, _ = strconv.ParseInt(at, 10, 64)
	}
	return client, nil
}

func (k *kafkaClient) fetch(deadline time.Time) ([][]byte, []int64, error) {
	_ = k.leader.conn.SetReadDeadline(deadline)
	return kafkaFetch(k.leader, k.topic, k.partition, k.offset, k.cfg.Limit())
}

func (k *kafkaClient) close() {
	_ = k.leader.conn.Close()
}

func executeKafka(cfg Config, req *Request) (*Response, error) {
	started := time.Now()
	client, err := kafkaOpen(cfg, req)
	if err != nil {
		return nil, err
	}
	defer client.close()
	want := wantMessages(req)
	deadline := time.Now().Add(windowDuration(cfg, req))
	messages := []map[string]any{}
	total := 0
	for len(messages) < want && time.Now().Before(deadline) {
		values, offsets, err := client.fetch(deadline)
		if err != nil {
			var timeout net.Error
			if errors.As(err, &timeout) && timeout.Timeout() {
				break
			}
			return nil, client.leader.redact(err.Error())
		}
		for i, value := range values {
			client.offset = offsets[i] + 1
			if err := addMessageBytes(&total, len(value), cfg.Limit()); err != nil {
				return nil, err
			}
			messages = append(messages, map[string]any{
				"partition": client.partition,
				"offset":    offsets[i],
				"payload":   payloadJSON(FromBytes(value)),
			})
			if len(messages) >= want {
				break
			}
		}
	}
	encoded, err := json.Marshal(map[string]any{"messages": messages})
	if err != nil {
		return nil, fmt.Errorf("kafka response could not be encoded: %v", err)
	}
	return &Response{
		Status: 0,
		MS:     time.Since(started).Milliseconds(),
		Headers: map[string]string{
			"x-topic":    client.topic,
			"x-messages": strconv.Itoa(len(messages)),
		},
		Body: Payload{Encoding: "utf8", Data: string(encoded)},
	}, nil
}

type kafkaStream struct {
	client  *kafkaClient
	pending [][]byte
}

func openKafkaStream(cfg Config, req *Request) (StreamSession, error) {
	client, err := kafkaOpen(cfg, req)
	if err != nil {
		return nil, err
	}
	return &kafkaStream{client: client}, nil
}

func (s *kafkaStream) ReadFrame() ([]byte, error) {
	for len(s.pending) == 0 {
		values, offsets, err := s.client.fetch(time.Now().Add(500 * time.Millisecond))
		if err != nil {
			var timeout net.Error
			if errors.As(err, &timeout) && timeout.Timeout() {
				continue
			}
			return nil, s.client.leader.redact(err.Error())
		}
		if len(values) == 0 {
			continue
		}
		s.client.offset = offsets[len(offsets)-1] + 1
		s.pending = values[1:]
		return values[0], nil
	}
	frame := s.pending[0]
	s.pending = s.pending[1:]
	return frame, nil
}

func (s *kafkaStream) Close() error {
	s.client.close()
	return nil
}

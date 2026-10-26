//go:build apidoc_debug_amqp || apidoc_debug_messaging || apidoc_debug_all

package debug

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"strconv"
	"time"
)

func init() {
	RegisterExecutor("amqp", executeAMQP)
	RegisterStream("amqp", openAMQPStream)
}

// AMQP 0-9-1 frame types.
const (
	amqpFrameMethod    = 1
	amqpFrameHeader    = 2
	amqpFrameBody      = 3
	amqpFrameHeartbeat = 8
	amqpFrameEnd       = 0xCE
)

// amqpConn is a minimal AMQP 0-9-1 client: connection negotiation, one
// channel, queue declare/consume and publisher confirms.
type amqpConn struct {
	conn     net.Conn
	limit    int
	frameMax int
	password string
}

func amqpShortString(value string) []byte {
	if len(value) > 255 {
		value = value[:255]
	}
	return append([]byte{byte(len(value))}, value...)
}

func amqpLongString(value string) []byte {
	out := make([]byte, 4, 4+len(value))
	binary.BigEndian.PutUint32(out, uint32(len(value)))
	return append(out, value...)
}

func amqpEmptyTable() []byte { return []byte{0, 0, 0, 0} }

func amqpMethod(class, method uint16, args ...[]byte) []byte {
	out := make([]byte, 4, 4+64)
	binary.BigEndian.PutUint16(out, class)
	binary.BigEndian.PutUint16(out[2:], method)
	for _, arg := range args {
		out = append(out, arg...)
	}
	return out
}

func (a *amqpConn) writeFrame(frameType byte, channel uint16, payload []byte) error {
	frame := make([]byte, 7, 8+len(payload))
	frame[0] = frameType
	binary.BigEndian.PutUint16(frame[1:], channel)
	binary.BigEndian.PutUint32(frame[3:], uint32(len(payload)))
	frame = append(frame, payload...)
	frame = append(frame, amqpFrameEnd)
	_, err := a.conn.Write(frame)
	return err
}

func (a *amqpConn) readFrame() (byte, uint16, []byte, error) {
	var head [7]byte
	if _, err := io.ReadFull(a.conn, head[:]); err != nil {
		return 0, 0, nil, err
	}
	frameType := head[0]
	channel := binary.BigEndian.Uint16(head[1:])
	size := int(binary.BigEndian.Uint32(head[3:]))
	if size > a.limit {
		return 0, 0, nil, fmt.Errorf("amqp frame exceeds max_bytes (%d)", a.limit)
	}
	payload := make([]byte, size)
	if _, err := io.ReadFull(a.conn, payload); err != nil {
		return 0, 0, nil, fmt.Errorf("amqp frame truncated: %v", err)
	}
	var end [1]byte
	if _, err := io.ReadFull(a.conn, end[:]); err != nil {
		return 0, 0, nil, fmt.Errorf("amqp frame truncated: %v", err)
	}
	if end[0] != amqpFrameEnd {
		return 0, 0, nil, fmt.Errorf("amqp frame end marker is invalid")
	}
	return frameType, channel, payload, nil
}

func (a *amqpConn) writeMethod(channel, class, method uint16, args ...[]byte) error {
	return a.writeFrame(amqpFrameMethod, channel, amqpMethod(class, method, args...))
}

func amqpMethodOf(payload []byte) (uint16, uint16, []byte, error) {
	if len(payload) < 4 {
		return 0, 0, nil, fmt.Errorf("amqp method frame is truncated")
	}
	return binary.BigEndian.Uint16(payload), binary.BigEndian.Uint16(payload[2:]), payload[4:], nil
}

// amqpReadMethod reads until the expected class/method arrives on channel.
// Peer Close frames become errors; heartbeats are ignored.
func (a *amqpConn) readMethod(channel, class, method uint16) ([]byte, error) {
	for {
		frameType, frameChannel, payload, err := a.readFrame()
		if err != nil {
			return nil, err
		}
		if frameType == amqpFrameHeartbeat {
			continue
		}
		if frameType != amqpFrameMethod {
			return nil, fmt.Errorf("amqp expected a method frame")
		}
		gotClass, gotMethod, args, err := amqpMethodOf(payload)
		if err != nil {
			return nil, err
		}
		if gotClass == 10 && gotMethod == 50 || gotClass == 20 && gotMethod == 40 {
			// Connection.Close / Channel.Close carry a reply code and text.
			if len(args) >= 2 {
				code := binary.BigEndian.Uint16(args)
				text, _ := amqpShortStringAt(args, 2)
				if gotClass == 10 {
					_ = a.writeMethod(frameChannel, 10, 51)
				} else {
					_ = a.writeMethod(frameChannel, 20, 41)
				}
				return nil, fmt.Errorf("amqp server closed the connection: %d %s", code, text)
			}
			return nil, fmt.Errorf("amqp server closed the connection")
		}
		if gotClass == 60 && gotMethod == 120 && frameChannel == channel {
			return nil, fmt.Errorf("amqp publish was rejected by the broker (nack)")
		}
		if gotClass == class && gotMethod == method && frameChannel == channel {
			return args, nil
		}
	}
}

func amqpShortStringAt(data []byte, pos int) (string, int) {
	if pos >= len(data) {
		return "", pos
	}
	length := int(data[pos])
	pos++
	if pos+length > len(data) {
		return "", len(data)
	}
	return string(data[pos : pos+length]), pos + length
}

func (a *amqpConn) handshake(cfg Config, req *Request) error {
	if _, err := a.conn.Write([]byte("AMQP\x00\x00\x09\x01")); err != nil {
		return fmt.Errorf("amqp connect failed: %v", err)
	}
	start, err := a.readMethod(0, 10, 10)
	if err != nil {
		return a.redact(fmt.Sprintf("amqp connect failed: %v", err))
	}
	if len(start) < 2 {
		return fmt.Errorf("amqp Start frame is truncated")
	}
	pos := 2
	pos = amqpSkipTable(start, pos) // server-properties
	mechanisms, pos := amqpLongStringAt(start, pos)
	if !containsToken(mechanisms, "PLAIN") {
		return fmt.Errorf("amqp server does not offer SASL PLAIN")
	}
	_ = pos

	user := req.Opts["username"]
	if user == "" {
		user = "guest"
	}
	password := req.Opts["password"]
	if password == "" {
		password = "guest"
	}
	response := "\x00" + user + "\x00" + password
	if err := a.writeMethod(0, 10, 11,
		amqpEmptyTable(), amqpShortString("PLAIN"), amqpLongString(response), amqpShortString("en_US"),
	); err != nil {
		return a.redact(fmt.Sprintf("amqp start failed: %v", err))
	}

	tune, err := a.readMethod(0, 10, 30)
	if err != nil {
		return a.redact(fmt.Sprintf("amqp connect failed: %v", err))
	}
	if len(tune) < 8 {
		return fmt.Errorf("amqp Tune frame is truncated")
	}
	serverFrameMax := int(binary.BigEndian.Uint32(tune[2:]))
	a.frameMax = 131072
	if serverFrameMax >= 4096 && serverFrameMax < a.frameMax {
		a.frameMax = serverFrameMax
	}
	if err := a.writeMethod(0, 10, 31,
		[]byte{0, 0},
		uint32Bytes(uint32(a.frameMax)),
		[]byte{0, 0},
	); err != nil {
		return a.redact(fmt.Sprintf("amqp tune failed: %v", err))
	}

	vhost := req.Opts["vhost"]
	if vhost == "" {
		vhost = "/"
	}
	if err := a.writeMethod(0, 10, 40, amqpShortString(vhost), amqpShortString(""), []byte{0}); err != nil {
		return a.redact(fmt.Sprintf("amqp open failed: %v", err))
	}
	if _, err := a.readMethod(0, 10, 41); err != nil {
		return a.redact(fmt.Sprintf("amqp connect failed: %v", err))
	}
	if err := a.writeMethod(1, 20, 10, amqpShortString("")); err != nil {
		return a.redact(fmt.Sprintf("amqp channel failed: %v", err))
	}
	if _, err := a.readMethod(1, 20, 11); err != nil {
		return a.redact(fmt.Sprintf("amqp channel failed: %v", err))
	}
	return nil
}

func containsToken(list, want string) bool {
	for _, item := range splitOnSpace(list) {
		if item == want {
			return true
		}
	}
	return false
}

func splitOnSpace(value string) []string {
	out := []string{}
	start := 0
	for i := 0; i <= len(value); i++ {
		if i == len(value) || value[i] == ' ' {
			if i > start {
				out = append(out, value[start:i])
			}
			start = i + 1
		}
	}
	return out
}

func amqpSkipTable(data []byte, pos int) int {
	if pos+4 > len(data) {
		return len(data)
	}
	size := int(binary.BigEndian.Uint32(data[pos:]))
	pos += 4 + size
	if pos > len(data) {
		return len(data)
	}
	return pos
}

func amqpLongStringAt(data []byte, pos int) (string, int) {
	if pos+4 > len(data) {
		return "", len(data)
	}
	size := int(binary.BigEndian.Uint32(data[pos:]))
	pos += 4
	if pos+size > len(data) {
		return "", len(data)
	}
	return string(data[pos : pos+size]), pos + size
}

func uint32Bytes(value uint32) []byte {
	out := make([]byte, 4)
	binary.BigEndian.PutUint32(out, value)
	return out
}

func (a *amqpConn) redact(text string) error {
	return fmt.Errorf("%s", redactSecrets(text, a.password))
}

func amqpConnect(cfg Config, req *Request) (*amqpConn, error) {
	host, port, scheme, err := parseMessagingTarget(req.Target, "amqp", "5672", map[string]bool{
		"amqp": true, "amqps": true,
	})
	if err != nil {
		return nil, err
	}
	if scheme == "amqps" {
		return nil, fmt.Errorf("amqp over TLS requires a pinned transport; use amqp:// (plaintext) in this build")
	}
	conn, err := dialMessaging(cfg, host, port, req.Timeout(cfg))
	if err != nil {
		return nil, fmt.Errorf("amqp %v", err)
	}
	client := &amqpConn{conn: conn, limit: cfg.Limit(), password: req.Opts["password"]}
	_ = conn.SetDeadline(time.Now().Add(req.Timeout(cfg)))
	if err := client.handshake(cfg, req); err != nil {
		_ = conn.Close()
		return nil, err
	}
	_ = conn.SetDeadline(time.Time{})
	return client, nil
}

func (a *amqpConn) enableConfirms() error {
	if err := a.writeMethod(1, 85, 10, []byte{0}); err != nil {
		return a.redact(fmt.Sprintf("amqp confirm select failed: %v", err))
	}
	if _, err := a.readMethod(1, 85, 11); err != nil {
		return a.redact(fmt.Sprintf("amqp confirm select failed: %v", err))
	}
	return nil
}

func (a *amqpConn) publish(req *Request) error {
	body, err := req.Body.Decode()
	if err != nil {
		return err
	}
	if len(body) > a.limit {
		return fmt.Errorf("request body exceeds max_bytes (%d)", a.limit)
	}
	routingKey := req.Opts["routing_key"]
	if routingKey == "" {
		return fmt.Errorf("amqp publish requires opts.routing_key")
	}
	exchange := req.Opts["exchange"]
	if err := a.enableConfirms(); err != nil {
		return err
	}
	if err := a.writeMethod(1, 60, 40,
		[]byte{0, 0}, amqpShortString(exchange), amqpShortString(routingKey), []byte{0},
	); err != nil {
		return a.redact(fmt.Sprintf("amqp publish failed: %v", err))
	}
	header := make([]byte, 14)
	binary.BigEndian.PutUint16(header, 60)
	binary.BigEndian.PutUint64(header[4:], uint64(len(body)))
	if err := a.writeFrame(amqpFrameHeader, 1, header); err != nil {
		return a.redact(fmt.Sprintf("amqp publish failed: %v", err))
	}
	chunk := a.frameMax - 8
	if chunk < 1 {
		chunk = 1
	}
	for offset := 0; offset < len(body); offset += chunk {
		end := offset + chunk
		if end > len(body) {
			end = len(body)
		}
		if err := a.writeFrame(amqpFrameBody, 1, body[offset:end]); err != nil {
			return a.redact(fmt.Sprintf("amqp publish failed: %v", err))
		}
	}
	if _, err := a.readMethod(1, 60, 80); err != nil {
		return a.redact(fmt.Sprintf("amqp publish confirmation failed: %v", err))
	}
	return nil
}

func (a *amqpConn) declareQueue(queue string) error {
	if err := a.writeMethod(1, 50, 10,
		[]byte{0, 0}, amqpShortString(queue), []byte{0}, amqpEmptyTable(),
	); err != nil {
		return a.redact(fmt.Sprintf("amqp queue declare failed: %v", err))
	}
	if _, err := a.readMethod(1, 50, 11); err != nil {
		return a.redact(fmt.Sprintf("amqp queue declare failed: %v", err))
	}
	return nil
}

func (a *amqpConn) consume(queue, tag string) error {
	if err := a.writeMethod(1, 60, 20,
		[]byte{0, 0}, amqpShortString(queue), amqpShortString(tag), []byte{0}, amqpEmptyTable(),
	); err != nil {
		return a.redact(fmt.Sprintf("amqp consume failed: %v", err))
	}
	if _, err := a.readMethod(1, 60, 21); err != nil {
		return a.redact(fmt.Sprintf("amqp consume failed: %v", err))
	}
	return nil
}

// amqpDelivery reads one Basic.Deliver plus its content header and body.
func (a *amqpConn) amqpDelivery(deadline time.Time) (map[string]any, error) {
	for {
		_ = a.conn.SetReadDeadline(deadline)
		frameType, channel, payload, err := a.readFrame()
		if err != nil {
			return nil, err
		}
		if frameType != amqpFrameMethod {
			continue
		}
		class, method, args, err := amqpMethodOf(payload)
		if err != nil {
			return nil, err
		}
		if class == 10 && method == 50 || class == 20 && method == 40 {
			if len(args) >= 2 {
				code := binary.BigEndian.Uint16(args)
				text, _ := amqpShortStringAt(args, 2)
				return nil, fmt.Errorf("amqp server closed the connection: %d %s", code, text)
			}
			return nil, fmt.Errorf("amqp server closed the connection")
		}
		if class != 60 || method != 60 || channel != 1 {
			continue
		}
		pos := 0
		_, pos = amqpShortStringAt(args, pos) // consumer tag
		if pos+8 > len(args) {
			return nil, fmt.Errorf("amqp delivery frame is truncated")
		}
		pos += 8 // delivery tag
		pos++    // redelivered bit
		exchange, pos := amqpShortStringAt(args, pos)
		routingKey, pos := amqpShortStringAt(args, pos)
		_ = pos

		// Content header: class, weight, body size, property flags.
		_, _, headerPayload, err := a.readFrame()
		if err != nil {
			return nil, err
		}
		if len(headerPayload) < 14 {
			return nil, fmt.Errorf("amqp content header is truncated")
		}
		bodySize := int(binary.BigEndian.Uint64(headerPayload[4:]))
		if bodySize > a.limit {
			return nil, fmt.Errorf("amqp message exceeds max_bytes (%d)", a.limit)
		}
		body := make([]byte, 0, bodySize)
		for len(body) < bodySize {
			frameType, _, chunk, err := a.readFrame()
			if err != nil {
				return nil, err
			}
			if frameType != amqpFrameBody {
				return nil, fmt.Errorf("amqp expected a body frame")
			}
			body = append(body, chunk...)
			if len(body) > a.limit {
				return nil, fmt.Errorf("amqp message exceeds max_bytes (%d)", a.limit)
			}
		}
		return map[string]any{
			"exchange":    exchange,
			"routing_key": routingKey,
			"payload":     payloadJSON(FromBytes(body)),
		}, nil
	}
}

func (a *amqpConn) close() {
	_ = a.conn.SetWriteDeadline(time.Now().Add(100 * time.Millisecond))
	_ = a.writeMethod(1, 20, 40, []byte{0, 200}, amqpShortString("apidoc done"), []byte{0, 0}, []byte{0, 0})
	_ = a.writeMethod(0, 10, 50, []byte{0, 200}, amqpShortString("apidoc done"), []byte{0, 0}, []byte{0, 0})
	_ = a.conn.Close()
}

func amqpConsumeSetup(client *amqpConn, req *Request) (string, error) {
	queue := req.Opts["queue"]
	if queue == "" {
		return "", fmt.Errorf("amqp consume requires opts.queue")
	}
	declare, err := optBool(req, "declare", false)
	if err != nil {
		return "", fmt.Errorf("invalid amqp declare; use \"true\" or \"false\"")
	}
	if declare {
		if err := client.declareQueue(queue); err != nil {
			return "", err
		}
	}
	tag := randomClientID("apidoc-")
	if err := client.consume(queue, tag); err != nil {
		return "", err
	}
	return tag, nil
}

func executeAMQP(cfg Config, req *Request) (*Response, error) {
	started := time.Now()
	consume, err := optBool(req, "consume", false)
	if err != nil {
		return nil, fmt.Errorf("invalid amqp consume; use \"true\" or \"false\"")
	}
	client, err := amqpConnect(cfg, req)
	if err != nil {
		return nil, err
	}
	defer client.close()

	var body map[string]any
	count := 0
	if consume {
		if _, err := amqpConsumeSetup(client, req); err != nil {
			return nil, err
		}
		want := wantMessages(req)
		deadline := time.Now().Add(windowDuration(cfg, req))
		deliveries := []map[string]any{}
		total := 0
		for len(deliveries) < want && time.Now().Before(deadline) {
			delivery, err := client.amqpDelivery(deadline)
			if err != nil {
				if ne, ok := err.(net.Error); ok && ne.Timeout() {
					break
				}
				return nil, client.redact(fmt.Sprintf("amqp consume failed: %v", err))
			}
			payload := delivery["payload"].(map[string]string)
			if err := addMessageBytes(&total, len(payload["data"]), cfg.Limit()); err != nil {
				return nil, err
			}
			deliveries = append(deliveries, delivery)
		}
		count = len(deliveries)
		if count == 1 {
			body = map[string]any{"delivery": deliveries[0]}
		} else {
			body = map[string]any{"deliveries": deliveries}
		}
	} else {
		if err := client.publish(req); err != nil {
			return nil, err
		}
		body = map[string]any{"published": true}
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("amqp response could not be encoded: %v", err)
	}
	return &Response{
		Status:  0,
		MS:      time.Since(started).Milliseconds(),
		Headers: map[string]string{"x-messages": strconv.Itoa(count)},
		Body:    Payload{Encoding: "utf8", Data: string(encoded)},
	}, nil
}

type amqpStream struct {
	client *amqpConn
}

func openAMQPStream(cfg Config, req *Request) (StreamSession, error) {
	client, err := amqpConnect(cfg, req)
	if err != nil {
		return nil, err
	}
	if _, err := amqpConsumeSetup(client, req); err != nil {
		client.close()
		return nil, err
	}
	return &amqpStream{client: client}, nil
}

// ReadFrame returns one delivery payload. Consuming is non-acknowledging:
// closing the session hands unacked messages back to the broker.
func (s *amqpStream) ReadFrame() ([]byte, error) {
	for {
		delivery, err := s.client.amqpDelivery(time.Time{})
		if err != nil {
			return nil, s.client.redact(err.Error())
		}
		payload := delivery["payload"].(map[string]string)
		return (&Payload{Encoding: payload["encoding"], Data: payload["data"]}).Decode()
	}
}

func (s *amqpStream) Close() error {
	s.client.close()
	return nil
}

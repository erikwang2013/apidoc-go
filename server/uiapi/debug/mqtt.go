//go:build apidoc_debug_mqtt || apidoc_debug_messaging || apidoc_debug_all

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
	RegisterExecutor("mqtt", executeMQTT)
	RegisterStream("mqtt", openMQTTStream)
}

// mqttConn reads MQTT 3.1.1 control packets over a checked TCP connection.
// Only the subset the relay needs is implemented: CONNECT/SUBSCRIBE/PUBLISH
// with QoS 0-1; QoS 2 is rejected instead of silently downgraded.
type mqttConn struct {
	conn  net.Conn
	limit int
}

func (m *mqttConn) writePacket(header byte, body []byte) error {
	packet := append([]byte{header}, mqttRemainingLength(len(body))...)
	packet = append(packet, body...)
	_, err := m.conn.Write(packet)
	return err
}

// readPacket returns the fixed header byte and the variable header/body.
func (m *mqttConn) readPacket() (byte, []byte, error) {
	var head [1]byte
	if _, err := io.ReadFull(m.conn, head[:]); err != nil {
		return 0, nil, err
	}
	length := 0
	multiplier := 1
	for i := 0; i < 4; i++ {
		var b [1]byte
		if _, err := io.ReadFull(m.conn, b[:]); err != nil {
			return 0, nil, err
		}
		length += int(b[0]&0x7f) * multiplier
		if b[0]&0x80 == 0 {
			break
		}
		multiplier *= 128
		if i == 3 {
			return 0, nil, fmt.Errorf("mqtt remaining length is malformed")
		}
	}
	if length > m.limit {
		return 0, nil, fmt.Errorf("mqtt packet exceeds max_bytes (%d)", m.limit)
	}
	body := make([]byte, length)
	if _, err := io.ReadFull(m.conn, body); err != nil {
		return 0, nil, fmt.Errorf("mqtt packet truncated: %v", err)
	}
	return head[0], body, nil
}

func mqttRemainingLength(n int) []byte {
	out := []byte{}
	for {
		digit := byte(n % 128)
		n /= 128
		if n > 0 {
			digit |= 0x80
		}
		out = append(out, digit)
		if n == 0 {
			return out
		}
	}
}

func mqttString(value string) []byte {
	out := make([]byte, 2, 2+len(value))
	binary.BigEndian.PutUint16(out, uint16(len(value)))
	return append(out, value...)
}

func mqttReadString(body []byte, pos *int) (string, error) {
	if *pos+2 > len(body) {
		return "", fmt.Errorf("mqtt string is truncated")
	}
	length := int(binary.BigEndian.Uint16(body[*pos:]))
	*pos += 2
	if *pos+length > len(body) {
		return "", fmt.Errorf("mqtt string is truncated")
	}
	value := string(body[*pos : *pos+length])
	*pos += length
	return value, nil
}

func mqttQoS(req *Request) (byte, error) {
	raw := req.Opts["qos"]
	switch raw {
	case "", "0":
		return 0, nil
	case "1":
		return 1, nil
	case "2":
		// shortcut: QoS 2 needs a two-phase PUBREC/PUBREL handshake; use QoS 1.
		return 0, fmt.Errorf("mqtt qos 2 is not supported; use 0 or 1")
	default:
		return 0, fmt.Errorf("invalid mqtt qos `%s`; use 0 or 1", raw)
	}
}

func mqttTopic(req *Request) (string, error) {
	topic := req.Opts["topic"]
	if topic == "" {
		return "", fmt.Errorf("mqtt requires opts.topic")
	}
	if len(topic) > 65535 {
		return "", fmt.Errorf("mqtt topic is too long")
	}
	return topic, nil
}

func mqttConnect(cfg Config, req *Request) (*mqttConn, error) {
	host, port, scheme, err := parseMessagingTarget(req.Target, "mqtt", "1883", map[string]bool{
		"mqtt": true, "tcp": true, "mqtts": true, "ssl": true,
	})
	if err != nil {
		return nil, err
	}
	if scheme == "mqtts" || scheme == "ssl" {
		return nil, fmt.Errorf("mqtt over TLS requires a pinned transport; use mqtt:// (plaintext) in this build")
	}
	conn, err := dialMessaging(cfg, host, port, req.Timeout(cfg))
	if err != nil {
		return nil, fmt.Errorf("mqtt %v", err)
	}
	client := &mqttConn{conn: conn, limit: cfg.Limit()}
	_ = conn.SetDeadline(time.Now().Add(req.Timeout(cfg)))
	if err := client.handshake(req); err != nil {
		_ = conn.Close()
		return nil, err
	}
	_ = conn.SetDeadline(time.Time{})
	return client, nil
}

func (m *mqttConn) handshake(req *Request) error {
	username, password := req.Opts["username"], req.Opts["password"]
	clientID := req.Opts["client_id"]
	if clientID == "" {
		clientID = randomClientID("apidoc-")
	}
	flags := byte(0x02) // clean session
	if username != "" || password != "" {
		flags |= 0x80
	}
	if password != "" {
		flags |= 0x40
	}
	body := mqttString("MQTT")
	body = append(body, 0x04, flags, 0x00, 0x00) // level 4, keepalive disabled (no PINGREQ)
	body = append(body, mqttString(clientID)...)
	if username != "" || password != "" {
		body = append(body, mqttString(username)...)
	}
	if password != "" {
		body = append(body, mqttString(password)...)
	}
	if err := m.writePacket(0x10, body); err != nil {
		return redactedError(fmt.Sprintf("mqtt connect failed: %v", err), password)
	}
	header, ack, err := m.readPacket()
	if err != nil {
		return redactedError(fmt.Sprintf("mqtt connect failed: %v", err), password)
	}
	if header>>4 != 2 || len(ack) < 2 {
		return fmt.Errorf("mqtt broker sent an unexpected CONNACK")
	}
	if code := ack[1]; code != 0 {
		return fmt.Errorf("mqtt broker rejected the connection: %s", mqttConnackText(code))
	}

	topic, err := mqttTopic(req)
	if err != nil {
		return err
	}
	qos, err := mqttQoS(req)
	if err != nil {
		return err
	}
	subscribe := []byte{0x00, 0x01} // packet id 1
	subscribe = append(subscribe, mqttString(topic)...)
	subscribe = append(subscribe, qos)
	if err := m.writePacket(0x82, subscribe); err != nil {
		return redactedError(fmt.Sprintf("mqtt subscribe failed: %v", err), password)
	}
	header, ack, err = m.readPacket()
	if err != nil {
		return redactedError(fmt.Sprintf("mqtt subscribe failed: %v", err), password)
	}
	if header>>4 != 9 || len(ack) < 3 {
		return fmt.Errorf("mqtt broker sent an unexpected SUBACK")
	}
	if ack[2] == 0x80 {
		return fmt.Errorf("mqtt broker rejected the subscription to `%s`", topic)
	}
	return nil
}

func mqttConnackText(code byte) string {
	switch code {
	case 1:
		return "unacceptable protocol version"
	case 2:
		return "identifier rejected"
	case 3:
		return "server unavailable"
	case 4:
		return "bad user name or password"
	case 5:
		return "not authorized"
	default:
		return fmt.Sprintf("error %d", code)
	}
}

func (m *mqttConn) publish(req *Request) error {
	body, err := req.Body.Decode()
	if err != nil {
		return err
	}
	if len(body) == 0 {
		return nil
	}
	if len(body) > m.limit {
		return fmt.Errorf("request body exceeds max_bytes (%d)", m.limit)
	}
	topic, err := mqttTopic(req)
	if err != nil {
		return err
	}
	qos, err := mqttQoS(req)
	if err != nil {
		return err
	}
	header := byte(0x30) | qos<<1
	packet := mqttString(topic)
	if qos > 0 {
		packet = append(packet, 0x00, 0x02) // packet id 2
	}
	packet = append(packet, body...)
	if writeErr := m.writePacket(header, packet); writeErr != nil {
		return redactedError(fmt.Sprintf("mqtt publish failed: %v", writeErr), req.Opts["password"])
	}
	return nil
}

// mqttMessage decodes one PUBLISH packet and acknowledges QoS 1.
func (m *mqttConn) mqttMessage(header byte, body []byte) (map[string]any, error) {
	qos := (header >> 1) & 0x03
	if qos == 2 {
		return nil, fmt.Errorf("mqtt qos 2 messages are not supported")
	}
	pos := 0
	topic, err := mqttReadString(body, &pos)
	if err != nil {
		return nil, err
	}
	packetID := 0
	if qos == 1 {
		if pos+2 > len(body) {
			return nil, fmt.Errorf("mqtt publish is truncated")
		}
		packetID = int(binary.BigEndian.Uint16(body[pos:]))
		pos += 2
	}
	payload := FromBytes(body[pos:])
	if qos == 1 {
		ack := []byte{byte(packetID >> 8), byte(packetID)}
		if err := m.writePacket(0x40, ack); err != nil {
			return nil, fmt.Errorf("mqtt ack failed: %v", err)
		}
	}
	return map[string]any{
		"topic":   topic,
		"qos":     int(qos),
		"payload": payloadJSON(payload),
	}, nil
}

func (m *mqttConn) disconnect() {
	_ = m.conn.SetWriteDeadline(time.Now().Add(100 * time.Millisecond))
	_ = m.writePacket(0xE0, nil)
	_ = m.conn.Close()
}

func executeMQTT(cfg Config, req *Request) (*Response, error) {
	started := time.Now()
	client, err := mqttConnect(cfg, req)
	if err != nil {
		return nil, err
	}
	defer client.disconnect()
	if err := client.publish(req); err != nil {
		return nil, err
	}

	want := wantMessages(req)
	deadline := time.Now().Add(windowDuration(cfg, req))
	messages := []map[string]any{}
	total := 0
	for len(messages) < want && time.Now().Before(deadline) {
		_ = client.conn.SetReadDeadline(deadline)
		header, body, err := client.readPacket()
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				break
			}
			return nil, redactedError(fmt.Sprintf("mqtt receive failed: %v", err), req.Opts["password"])
		}
		if header>>4 != 3 {
			continue
		}
		message, err := client.mqttMessage(header, body)
		if err != nil {
			return nil, err
		}
		payload := message["payload"].(map[string]string)
		if err := addMessageBytes(&total, len(payload["data"]), cfg.Limit()); err != nil {
			return nil, err
		}
		messages = append(messages, message)
	}
	encoded, err := json.Marshal(map[string]any{"messages": messages})
	if err != nil {
		return nil, fmt.Errorf("mqtt response could not be encoded: %v", err)
	}
	return &Response{
		Status: 0,
		MS:     time.Since(started).Milliseconds(),
		Headers: map[string]string{
			"x-topic":    req.Opts["topic"],
			"x-messages": strconv.Itoa(len(messages)),
		},
		Body: Payload{Encoding: "utf8", Data: string(encoded)},
	}, nil
}

type mqttStream struct {
	client *mqttConn
}

func openMQTTStream(cfg Config, req *Request) (StreamSession, error) {
	client, err := mqttConnect(cfg, req)
	if err != nil {
		return nil, err
	}
	if err := client.publish(req); err != nil {
		client.disconnect()
		return nil, err
	}
	return &mqttStream{client: client}, nil
}

// ReadFrame returns one PUBLISH payload. Control packets are consumed
// silently; QoS 1 messages are acknowledged before delivery.
func (s *mqttStream) ReadFrame() ([]byte, error) {
	for {
		header, body, err := s.client.readPacket()
		if err != nil {
			return nil, err
		}
		if header>>4 != 3 {
			continue
		}
		message, err := s.client.mqttMessage(header, body)
		if err != nil {
			return nil, err
		}
		payload, ok := message["payload"].(map[string]string)
		if !ok {
			return nil, fmt.Errorf("mqtt payload is malformed")
		}
		return (&Payload{Encoding: payload["encoding"], Data: payload["data"]}).Decode()
	}
}

func (s *mqttStream) Close() error {
	s.client.disconnect()
	return nil
}

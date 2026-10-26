//go:build apidoc_debug_industrial || apidoc_debug_all

package debug

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"net"
	"time"
)

const (
	opcuaDefaultPort  = "4840"
	opcuaNonePolicy   = "http://opcfoundation.org/UA/SecurityPolicy#None"
	opcuaAnonymous    = 321
	opcuaUsername     = 324
	opcuaOpenChannel  = 446
	opcuaCreateSess   = 461
	opcuaActivateSess = 467
	opcuaBrowse       = 527
	opcuaRead         = 631
	opcuaWrite        = 673
)

func init() {
	RegisterExecutor("opcua", executeOPCUA)
}

// OPC UA Binary minimal client over SecurityPolicy None: Browse, Read and
// Write with an anonymous or username token. Encrypted/signed channels,
// subscriptions, history reads, multi-chunk responses and CloseSession are
// out of scope. Credentials only go into requests, never into errors or the
// response.
func executeOPCUA(cfg Config, req *Request) (*Response, error) {
	started := time.Now()
	host, port, err := parseProtocolTarget(req.Target, "opc.tcp", opcuaDefaultPort, "opc.tcp")
	if err != nil {
		return nil, err
	}
	operation := req.Opts["operation"]
	if operation == "" {
		operation = "read"
	}
	switch operation {
	case "read", "browse", "write":
	default:
		return nil, fmt.Errorf("unsupported opcua operation `%s`; use browse, read or write", operation)
	}
	if operation == "write" {
		if _, found := req.Opts["value"]; !found {
			return nil, fmt.Errorf("opcua `value` is required for write")
		}
	}
	nodeID := req.Opts["node_id"]
	if nodeID == "" {
		nodeID = "i=85"
	}
	if _, err := opcuaPutNodeIDString(nil, nodeID); err != nil {
		return nil, err // validate before connecting; invalid ids never reach the server
	}
	attribute := uint32(protocolOptUint(req, "attribute", 13)) // 13 = Value
	endpoint := "opc.tcp://" + net.JoinHostPort(host, port)

	client, err := opcuaConnect(cfg, host, port, endpoint)
	if err != nil {
		return nil, err
	}
	defer client.conn.Close()
	username, hasUsername := req.Opts["username"]
	tokenPolicy := req.Opts["user_token_policy"]
	if tokenPolicy == "" {
		tokenPolicy = "username"
	}
	if err := client.createSession(username, hasUsername, req.Opts["password"], tokenPolicy); err != nil {
		return nil, err
	}

	var body map[string]any
	switch operation {
	case "read":
		value, err := client.read(nodeID, attribute)
		if err != nil {
			return nil, err
		}
		body = map[string]any{"value": value, "status": "Good"}
	case "browse":
		references, err := client.browse(nodeID)
		if err != nil {
			return nil, err
		}
		body = map[string]any{"references": references, "status": "Good"}
	case "write":
		if err := client.write(nodeID, attribute, req.Opts["value"]); err != nil {
			return nil, err
		}
		body = map[string]any{"status": "Good"}
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("encode response: %v", err)
	}
	return &Response{
		Status: 0,
		MS:     time.Since(started).Milliseconds(),
		Headers: map[string]string{
			"x-peer":         client.conn.RemoteAddr().String(),
			"x-opcua-status": "Good",
		},
		Body: FromBytes(encoded),
	}, nil
}

type opcuaClient struct {
	conn        net.Conn
	channel     uint32
	token       uint32
	seq         uint32
	request     uint32
	auth        []byte
	endpointURL string
	deadline    time.Time
	maxBytes    int
}

func opcuaConnect(cfg Config, host, port, endpoint string) (*opcuaClient, error) {
	timeout := cfg.Timeout()
	conn, err := dialPinned(cfg, "tcp", host, port, timeout)
	if err != nil {
		return nil, err
	}
	client := &opcuaClient{
		conn:        conn,
		seq:         1,
		request:     1,
		auth:        []byte{0x00, 0x00},
		endpointURL: endpoint,
		deadline:    time.Now().Add(timeout),
		maxBytes:    cfg.Limit(),
	}
	maxMessage := cfg.Limit()
	if maxMessage > math.MaxUint32 {
		maxMessage = math.MaxUint32
	}
	if err := client.send(opcuaEncodeHEL(endpoint, uint32(maxMessage))); err != nil {
		conn.Close()
		return nil, err
	}
	ack, err := client.readMessage()
	if err != nil {
		conn.Close()
		return nil, err
	}
	if string(ack.kind[:]) != "ACK" {
		conn.Close()
		return nil, fmt.Errorf("opcua expected ACK, got %s", string(ack.kind[:]))
	}
	if err := client.openSecureChannel(); err != nil {
		conn.Close()
		return nil, err
	}
	return client, nil
}

func (c *opcuaClient) nextSeq() uint32 {
	value := c.seq
	c.seq++
	return value
}

func (c *opcuaClient) nextRequest() uint32 {
	value := c.request
	c.request++
	return value
}

func (c *opcuaClient) send(bytes []byte) error {
	if !time.Now().Before(c.deadline) {
		return fmt.Errorf("opcua request timed out")
	}
	if err := c.conn.SetWriteDeadline(c.deadline); err != nil {
		return fmt.Errorf("opcua write failed: %v", err)
	}
	if _, err := c.conn.Write(bytes); err != nil {
		if ne, ok := err.(net.Error); ok && ne.Timeout() {
			return fmt.Errorf("opcua write timed out")
		}
		return fmt.Errorf("opcua write failed: %v", err)
	}
	return nil
}

func (c *opcuaClient) readMessage() (*opcuaMessage, error) {
	return opcuaReadMessage(c.conn, c.deadline, c.maxBytes)
}

func (c *opcuaClient) openSecureChannel() error {
	var body []byte
	body = opcuaPutNodeID(body, opcuaOpenChannel)
	body = opcuaPutRequestHeader(body, []byte{0x00, 0x00}, 1)
	body = binary.LittleEndian.AppendUint32(body, 0) // clientProtocolVersion
	body = binary.LittleEndian.AppendUint32(body, 0) // requestType = Issue
	body = binary.LittleEndian.AppendUint32(body, 1) // securityMode = None
	body = opcuaPutBytes(body, nil)                  // clientNonce
	body = binary.LittleEndian.AppendUint32(body, 3600000)
	seq := c.nextSeq()
	request := c.nextRequest()
	message := opcuaEncodeOPN(c.channel, opcuaNonePolicy, seq, request, body)
	if err := c.send(message); err != nil {
		return err
	}
	response, err := c.readMessage()
	if err != nil {
		return err
	}
	if string(response.kind[:]) != "OPN" {
		return fmt.Errorf("opcua expected OPN, got %s", string(response.kind[:]))
	}
	if !response.hasPolicy || response.policy != opcuaNonePolicy {
		return fmt.Errorf("opcua server selected security policy `%s`; only None is supported", response.policy)
	}
	c.channel = response.channel
	dec := opcuaDecoder{data: response.body}
	if err := opcuaSkipNodeID(&dec); err != nil { // OpenSecureChannelResponse
		return err
	}
	if err := opcuaCheckResponseHeader(&dec); err != nil {
		return err
	}
	if _, err := dec.u32(); err != nil { // serverProtocolVersion
		return err
	}
	if _, err := dec.u32(); err != nil { // securityToken.channelId
		return err
	}
	token, err := dec.u32()
	if err != nil {
		return err
	}
	c.token = token
	return nil
}

func (c *opcuaClient) createSession(username string, hasUsername bool, password, tokenPolicy string) error {
	var body []byte
	body = opcuaPutNodeID(body, opcuaCreateSess)
	body = opcuaPutRequestHeader(body, c.auth, 2)
	body = opcuaPutString(body, "urn:apidoc:debug")
	body = opcuaPutString(body, "urn:apidoc")
	body = opcuaPutLocalizedText(body, "apidoc-debug")
	body = binary.LittleEndian.AppendUint32(body, 0) // ApplicationType = Client
	body = opcuaPutBytes(body, nil)                  // gatewayServerUri
	body = opcuaPutBytes(body, nil)                  // discoveryProfileUri
	body = binary.LittleEndian.AppendUint32(body, 0xFFFFFFFF)
	body = opcuaPutBytes(body, nil) // serverUri
	body = opcuaPutString(body, c.endpointURL)
	body = opcuaPutString(body, "apidoc-debug")
	body = opcuaPutBytes(body, nil) // clientNonce
	body = opcuaPutBytes(body, nil) // clientCertificate
	body = binary.LittleEndian.AppendUint64(body, math.Float64bits(600000))
	body = binary.LittleEndian.AppendUint32(body, 0) // maxResponseMessageSize
	response, err := c.roundtrip([3]byte{'M', 'S', 'G'}, body)
	if err != nil {
		return err
	}
	dec := opcuaDecoder{data: response}
	if err := opcuaSkipNodeID(&dec); err != nil {
		return err
	}
	if err := opcuaCheckResponseHeader(&dec); err != nil {
		return err
	}
	if err := opcuaSkipNodeID(&dec); err != nil { // sessionId
		return err
	}
	start := dec.pos
	if err := opcuaSkipNodeID(&dec); err != nil { // authenticationToken
		return err
	}
	c.auth = append([]byte{}, dec.data[start:dec.pos]...)
	return c.activateSession(username, hasUsername, password, tokenPolicy)
}

func (c *opcuaClient) activateSession(username string, hasUsername bool, password, tokenPolicy string) error {
	var body []byte
	body = opcuaPutNodeID(body, opcuaActivateSess)
	body = opcuaPutRequestHeader(body, c.auth, 3)
	body = opcuaPutBytes(body, nil)                           // clientSignature.algorithm
	body = opcuaPutBytes(body, nil)                           // clientSignature.signature
	body = binary.LittleEndian.AppendUint32(body, 0xFFFFFFFF) // clientSoftwareCertificates
	body = binary.LittleEndian.AppendUint32(body, 0xFFFFFFFF) // localeIds
	if hasUsername {
		var token []byte
		token = opcuaPutString(token, tokenPolicy)
		token = opcuaPutString(token, username)
		token = opcuaPutBytes(token, []byte(password))
		token = opcuaPutBytes(token, nil)
		body = opcuaPutExtensionObject(body, opcuaUsername, token)
	} else {
		body = opcuaPutExtensionObject(body, opcuaAnonymous, opcuaPutString(nil, "anonymous"))
	}
	body = opcuaPutBytes(body, nil) // userTokenSignature.algorithm
	body = opcuaPutBytes(body, nil) // userTokenSignature.signature
	response, err := c.roundtrip([3]byte{'M', 'S', 'G'}, body)
	if err != nil {
		return err
	}
	dec := opcuaDecoder{data: response}
	if err := opcuaSkipNodeID(&dec); err != nil { // ActivateSessionResponse
		return err
	}
	return opcuaCheckResponseHeader(&dec)
}

func (c *opcuaClient) read(nodeID string, attribute uint32) (any, error) {
	var body []byte
	body = opcuaPutNodeID(body, opcuaRead)
	body = opcuaPutRequestHeader(body, c.auth, 4)
	body = binary.LittleEndian.AppendUint64(body, 0) // maxAge
	body = binary.LittleEndian.AppendUint32(body, 0) // timestampsToReturn = Source
	body = binary.LittleEndian.AppendUint32(body, 1)
	body, err := opcuaPutNodeIDString(body, nodeID)
	if err != nil {
		return nil, err
	}
	body = binary.LittleEndian.AppendUint32(body, attribute)
	body = opcuaPutBytes(body, nil) // indexRange
	body = binary.LittleEndian.AppendUint16(body, 0)
	body = opcuaPutBytes(body, nil) // dataEncoding.name
	response, err := c.roundtrip([3]byte{'M', 'S', 'G'}, body)
	if err != nil {
		return nil, err
	}
	dec := opcuaDecoder{data: response}
	if err := opcuaSkipNodeID(&dec); err != nil { // ReadResponse
		return nil, err
	}
	if err := opcuaCheckResponseHeader(&dec); err != nil {
		return nil, err
	}
	count, err := dec.i32()
	if err != nil {
		return nil, err
	}
	if count != 1 {
		return nil, fmt.Errorf("opcua read returned %d results (expected 1)", count)
	}
	return opcuaDecodeDataValue(&dec)
}

func (c *opcuaClient) browse(nodeID string) ([]any, error) {
	var body []byte
	body = opcuaPutNodeID(body, opcuaBrowse)
	body = opcuaPutRequestHeader(body, c.auth, 5)
	body = opcuaPutNodeID(body, 0)                   // viewId null
	body = binary.LittleEndian.AppendUint64(body, 0) // timestamp
	body = binary.LittleEndian.AppendUint32(body, 0) // viewVersion
	body = binary.LittleEndian.AppendUint32(body, 0) // requestedMaxReferencesPerNode
	body = binary.LittleEndian.AppendUint32(body, 1)
	body, err := opcuaPutNodeIDString(body, nodeID)
	if err != nil {
		return nil, err
	}
	body = binary.LittleEndian.AppendUint32(body, 0) // browseDirection = Forward
	body = opcuaPutNodeID(body, 0)                   // referenceTypeId = null (all)
	body = append(body, 1)                           // includeSubtypes
	body = binary.LittleEndian.AppendUint32(body, 0) // nodeClassMask
	body = binary.LittleEndian.AppendUint32(body, 0x3F)
	response, err := c.roundtrip([3]byte{'M', 'S', 'G'}, body)
	if err != nil {
		return nil, err
	}
	dec := opcuaDecoder{data: response}
	if err := opcuaSkipNodeID(&dec); err != nil { // BrowseResponse
		return nil, err
	}
	if err := opcuaCheckResponseHeader(&dec); err != nil {
		return nil, err
	}
	count, err := dec.i32()
	if err != nil {
		return nil, err
	}
	if count != 1 {
		return nil, fmt.Errorf("opcua browse returned %d results (expected 1)", count)
	}
	status, err := dec.u32()
	if err != nil {
		return nil, err
	}
	if status != 0 {
		return nil, fmt.Errorf("opcua browse status %s", opcuaStatusLabel(status))
	}
	if _, _, err := dec.byteString(); err != nil { // continuation point
		return nil, err
	}
	references, err := dec.i32()
	if err != nil {
		return nil, err
	}
	out := []any{}
	if references < 0 {
		return out, nil
	}
	for i := int32(0); i < references && i < 10000; i++ {
		if err := opcuaSkipNodeID(&dec); err != nil { // referenceTypeId
			return nil, err
		}
		if _, err := dec.u8(); err != nil { // isForward
			return nil, err
		}
		target, err := opcuaDecodeNodeID(&dec)
		if err != nil {
			return nil, err
		}
		namespace, err := dec.u16()
		if err != nil {
			return nil, err
		}
		name, _, err := dec.string()
		if err != nil {
			return nil, err
		}
		display, err := opcuaDecodeLocalizedText(&dec)
		if err != nil {
			return nil, err
		}
		nodeClass, err := dec.u32()
		if err != nil {
			return nil, err
		}
		if err := opcuaSkipNodeID(&dec); err != nil { // typeDefinition
			return nil, err
		}
		out = append(out, map[string]any{
			"node_id":      target,
			"browse_name":  fmt.Sprintf("ns=%d;%s", namespace, name),
			"display_name": display,
			"node_class":   nodeClass,
		})
	}
	return out, nil
}

func (c *opcuaClient) write(nodeID string, attribute uint32, value string) error {
	var body []byte
	body = opcuaPutNodeID(body, opcuaWrite)
	body = opcuaPutRequestHeader(body, c.auth, 6)
	body = binary.LittleEndian.AppendUint32(body, 1)
	body, err := opcuaPutNodeIDString(body, nodeID)
	if err != nil {
		return err
	}
	body = binary.LittleEndian.AppendUint32(body, attribute)
	body = opcuaPutBytes(body, nil) // indexRange
	body = append(body, 0x01)       // DataValue encoding mask: value only
	body = opcuaEncodeVariant(body, value)
	response, err := c.roundtrip([3]byte{'M', 'S', 'G'}, body)
	if err != nil {
		return err
	}
	dec := opcuaDecoder{data: response}
	if err := opcuaSkipNodeID(&dec); err != nil { // WriteResponse
		return err
	}
	if err := opcuaCheckResponseHeader(&dec); err != nil {
		return err
	}
	count, err := dec.i32()
	if err != nil {
		return err
	}
	if count != 1 {
		return fmt.Errorf("opcua write returned %d results (expected 1)", count)
	}
	status, err := dec.u32()
	if err != nil {
		return err
	}
	if status != 0 {
		return fmt.Errorf("opcua write status %s", opcuaStatusLabel(status))
	}
	return nil
}

func (c *opcuaClient) roundtrip(kind [3]byte, body []byte) ([]byte, error) {
	seq := c.nextSeq()
	request := c.nextRequest()
	message := opcuaEncodeMSG(kind, c.channel, c.token, seq, request, body)
	if err := c.send(message); err != nil {
		return nil, err
	}
	response, err := c.readMessage()
	if err != nil {
		return nil, err
	}
	if response.kind != kind {
		return nil, fmt.Errorf("opcua expected %s, got %s", string(kind[:]), string(response.kind[:]))
	}
	return response.body, nil
}

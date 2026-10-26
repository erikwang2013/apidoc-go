//go:build apidoc_debug_xmpp || apidoc_debug_messaging || apidoc_debug_all

package debug

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

func init() {
	RegisterExecutor("xmpp", executeXMPP)
	RegisterStream("xmpp", openXMPPStream)
}

const (
	xmppStreamNS  = "http://etherx.jabber.org/streams"
	xmppClientNS  = "jabber:client"
	xmppTLSNS     = "urn:ietf:params:xml:ns:xmpp-tls"
	xmppSASLNS    = "urn:ietf:params:xml:ns:xmpp-sasl"
	xmppBindNS    = "urn:ietf:params:xml:ns:xmpp-bind"
	xmppSessionNS = "urn:ietf:params:xml:ns:xmpp-session"
	xmppStanzaNS  = "urn:ietf:params:xml:ns:xmpp-stanzas"
)

// xmppRootCAs overrides the system roots for STARTTLS; tests inject a local CA
// here. nil means the system roots.
var xmppRootCAs *x509.CertPool

type xmppJID struct {
	local    string
	domain   string
	resource string
}

func parseXMPPJID(raw string) (xmppJID, error) {
	if raw == "" {
		return xmppJID{}, fmt.Errorf("xmpp requires opts.jid")
	}
	if len(raw) > 1024 {
		return xmppJID{}, fmt.Errorf("xmpp jid is too long")
	}
	if strings.ContainsFunc(raw, func(r rune) bool { return unicode.IsSpace(r) || r < 0x20 }) {
		return xmppJID{}, fmt.Errorf("invalid xmpp jid")
	}
	at := strings.IndexByte(raw, '@')
	if at <= 0 {
		return xmppJID{}, fmt.Errorf("invalid xmpp jid")
	}
	local := raw[:at]
	rest := raw[at+1:]
	domain, resource := rest, ""
	if slash := strings.IndexByte(rest, '/'); slash >= 0 {
		domain, resource = rest[:slash], rest[slash+1:]
	}
	if local == "" || domain == "" || strings.Contains(domain, "@") {
		return xmppJID{}, fmt.Errorf("invalid xmpp jid")
	}
	return xmppJID{local: local, domain: domain, resource: resource}, nil
}

func xmppEscape(value string) string {
	var out bytes.Buffer
	_ = xml.EscapeText(&out, []byte(value))
	return out.String()
}

type xmppNode struct {
	name     xml.Name
	attrs    []xml.Attr
	text     strings.Builder
	children []*xmppNode
}

func (n *xmppNode) attr(name string) string {
	for _, attr := range n.attrs {
		if attr.Name.Local == name {
			return attr.Value
		}
	}
	return ""
}

func (n *xmppNode) child(space, local string) *xmppNode {
	for _, child := range n.children {
		if child.name.Local == local && (space == "" || child.name.Space == space) {
			return child
		}
	}
	return nil
}

func (n *xmppNode) find(space, local string) *xmppNode {
	if n.name.Local == local && (space == "" || n.name.Space == space) {
		return n
	}
	for _, child := range n.children {
		if found := child.find(space, local); found != nil {
			return found
		}
	}
	return nil
}

type xmppLimitReader struct {
	r    io.Reader
	left int
}

func (l *xmppLimitReader) Read(p []byte) (int, error) {
	if l.left <= 0 {
		return 0, fmt.Errorf("xmpp stanza exceeds max_bytes")
	}
	if len(p) > l.left {
		p = p[:l.left]
	}
	n, err := l.r.Read(p)
	l.left -= n
	return n, err
}

type xmppSession struct {
	conn     net.Conn
	dec      *xml.Decoder
	limiter  *xmppLimitReader
	limit    int
	local    string
	domain   string
	password string
}

func (s *xmppSession) reset(reader io.Reader) {
	s.limiter = &xmppLimitReader{r: reader, left: s.limit + 4096}
	s.dec = xml.NewDecoder(s.limiter)
}

func (s *xmppSession) write(deadline time.Time, text string) error {
	_ = s.conn.SetWriteDeadline(deadline)
	_, err := io.WriteString(s.conn, text)
	return err
}

func (s *xmppSession) writeStreamHeader(deadline time.Time) error {
	return s.write(deadline, `<?xml version="1.0"?>`+
		`<stream:stream to="`+xmppEscape(s.domain)+`" xmlns="`+xmppClientNS+`"`+
		` xmlns:stream="`+xmppStreamNS+`" version="1.0">`)
}

// readStreamStart consumes the server's stream header start tag only; the
// stream element stays open for the whole connection.
func (s *xmppSession) readStreamStart(deadline time.Time) error {
	s.limiter.left = s.limit + 4096
	_ = s.conn.SetReadDeadline(deadline)
	for {
		token, err := s.dec.Token()
		if err != nil {
			return err
		}
		if start, ok := token.(xml.StartElement); ok {
			if start.Name.Local == "stream" {
				return nil
			}
			return fmt.Errorf("xmpp server sent an unexpected element `%s`", start.Name.Local)
		}
	}
}

// readElement returns the next complete stanza (child of the stream).
func (s *xmppSession) readElement(deadline time.Time) (*xmppNode, error) {
	s.limiter.left = s.limit + 4096
	_ = s.conn.SetReadDeadline(deadline)
	for {
		token, err := s.dec.Token()
		if err != nil {
			return nil, err
		}
		start, ok := token.(xml.StartElement)
		if !ok {
			continue
		}
		root := &xmppNode{name: start.Name, attrs: start.Attr}
		stack := []*xmppNode{root}
		for len(stack) > 0 {
			token, err := s.dec.Token()
			if err != nil {
				return nil, err
			}
			switch value := token.(type) {
			case xml.StartElement:
				child := &xmppNode{name: value.Name, attrs: value.Attr}
				parent := stack[len(stack)-1]
				parent.children = append(parent.children, child)
				stack = append(stack, child)
			case xml.EndElement:
				stack = stack[:len(stack)-1]
			case xml.CharData:
				stack[len(stack)-1].text.Write(value)
			}
		}
		return root, nil
	}
}

func xmppErrorText(node *xmppNode) string {
	if text := node.find(xmppStanzaNS, "text"); text != nil {
		return strings.TrimSpace(text.text.String())
	}
	if text := node.child(xmppStreamNS, "text"); text != nil {
		return strings.TrimSpace(text.text.String())
	}
	return strings.TrimSpace(node.text.String())
}

func (s *xmppSession) startTLS(deadline time.Time) error {
	if err := s.writeStreamHeader(deadline); err != nil {
		return fmt.Errorf("xmpp connect failed: %v", err)
	}
	if err := s.readStreamStart(deadline); err != nil {
		return fmt.Errorf("xmpp connect failed: %v", err)
	}
	features, err := s.readElement(deadline)
	if err != nil {
		return fmt.Errorf("xmpp connect failed: %v", err)
	}
	if features.name.Local == "error" {
		return fmt.Errorf("xmpp stream error: %s", xmppErrorText(features))
	}
	if features.name.Local != "features" {
		return fmt.Errorf("xmpp server sent no stream features")
	}
	startTLS := features.find(xmppTLSNS, "starttls")
	if startTLS == nil {
		return fmt.Errorf("xmpp server does not offer STARTTLS")
	}
	if err := s.write(deadline, `<starttls xmlns="`+xmppTLSNS+`"/>`); err != nil {
		return fmt.Errorf("xmpp STARTTLS request failed: %v", err)
	}
	reply, err := s.readElement(deadline)
	if err != nil {
		return fmt.Errorf("xmpp STARTTLS failed: %v", err)
	}
	if reply.name.Local == "failure" {
		return fmt.Errorf("xmpp STARTTLS was refused by the server")
	}
	if reply.name.Local != "proceed" || reply.name.Space != xmppTLSNS {
		return fmt.Errorf("xmpp server sent an unexpected STARTTLS reply")
	}
	tlsConn := tls.Client(s.conn, &tls.Config{
		ServerName: s.domain,
		RootCAs:    xmppRootCAs,
		MinVersion: tls.VersionTLS12,
	})
	_ = s.conn.SetDeadline(deadline)
	if err := tlsConn.Handshake(); err != nil {
		return redactErr(fmt.Errorf("xmpp STARTTLS handshake failed: %v", err), s.password)
	}
	_ = tlsConn.SetDeadline(time.Time{})
	s.conn = tlsConn
	s.reset(tlsConn)
	return nil
}

func (s *xmppSession) authenticate(deadline time.Time) (*xmppNode, error) {
	if err := s.writeStreamHeader(deadline); err != nil {
		return nil, fmt.Errorf("xmpp connect failed: %v", err)
	}
	if err := s.readStreamStart(deadline); err != nil {
		return nil, fmt.Errorf("xmpp connect failed: %v", err)
	}
	features, err := s.readElement(deadline)
	if err != nil {
		return nil, fmt.Errorf("xmpp connect failed: %v", err)
	}
	if features.name.Local == "error" {
		return nil, fmt.Errorf("xmpp stream error: %s", xmppErrorText(features))
	}
	plain := false
	if mechanisms := features.find(xmppSASLNS, "mechanisms"); mechanisms != nil {
		for _, mechanism := range mechanisms.children {
			if mechanism.name.Local == "mechanism" && strings.TrimSpace(mechanism.text.String()) == "PLAIN" {
				plain = true
			}
		}
	}
	if !plain {
		return nil, fmt.Errorf("xmpp server does not offer SASL PLAIN")
	}
	raw := []byte("\x00" + s.local + "\x00" + s.password)
	auth := `<auth xmlns="` + xmppSASLNS + `" mechanism="PLAIN">` +
		base64.StdEncoding.EncodeToString(raw) + `</auth>`
	if err := s.write(deadline, auth); err != nil {
		return nil, redactErr(fmt.Errorf("xmpp SASL authentication failed: %v", err), s.password)
	}
	reply, err := s.readElement(deadline)
	if err != nil {
		return nil, redactErr(fmt.Errorf("xmpp SASL authentication failed: %v", err), s.password)
	}
	if reply.name.Local != "success" || reply.name.Space != xmppSASLNS {
		text := xmppErrorText(reply)
		if text == "" {
			return nil, fmt.Errorf("xmpp SASL authentication failed")
		}
		return nil, redactErr(fmt.Errorf("xmpp SASL authentication failed: %s", text), s.password)
	}
	s.reset(s.conn)
	if err := s.writeStreamHeader(deadline); err != nil {
		return nil, fmt.Errorf("xmpp connect failed: %v", err)
	}
	if err := s.readStreamStart(deadline); err != nil {
		return nil, fmt.Errorf("xmpp connect failed: %v", err)
	}
	features, err = s.readElement(deadline)
	if err != nil {
		return nil, fmt.Errorf("xmpp connect failed: %v", err)
	}
	return features, nil
}

func (s *xmppSession) bind(features *xmppNode, deadline time.Time) error {
	if err := s.write(deadline, `<iq type="set" id="bind-1"><bind xmlns="`+xmppBindNS+`"/></iq>`); err != nil {
		return fmt.Errorf("xmpp bind failed: %v", err)
	}
	bound := false
	for !bound {
		reply, err := s.readElement(deadline)
		if err != nil {
			return fmt.Errorf("xmpp bind failed: %v", err)
		}
		if reply.name.Local == "error" && reply.name.Space == xmppStreamNS {
			return fmt.Errorf("xmpp stream error: %s", xmppErrorText(reply))
		}
		if reply.name.Local != "iq" || reply.attr("id") != "bind-1" {
			continue
		}
		if reply.attr("type") == "error" {
			return fmt.Errorf("xmpp bind was rejected by the server")
		}
		jid := reply.find(xmppBindNS, "jid")
		if jid == nil || strings.TrimSpace(jid.text.String()) == "" {
			return fmt.Errorf("xmpp bind did not return a jid")
		}
		bound = true
	}
	if features.find(xmppSessionNS, "session") == nil {
		return nil
	}
	if err := s.write(deadline, `<iq type="set" id="session-1"><session xmlns="`+xmppSessionNS+`"/></iq>`); err != nil {
		return fmt.Errorf("xmpp session failed: %v", err)
	}
	for {
		reply, err := s.readElement(deadline)
		if err != nil {
			return fmt.Errorf("xmpp session failed: %v", err)
		}
		if reply.name.Local != "iq" || reply.attr("id") != "session-1" {
			continue
		}
		if reply.attr("type") == "error" {
			return fmt.Errorf("xmpp session was rejected by the server")
		}
		return nil
	}
}

type xmppRequest struct {
	jid         xmppJID
	to          string
	messageType string
	body        []byte
}

func parseXMPPRequest(req *Request) (*xmppRequest, error) {
	jid, err := parseXMPPJID(req.Opts["jid"])
	if err != nil {
		return nil, err
	}
	if req.Opts["password"] == "" {
		return nil, fmt.Errorf("xmpp requires opts.password")
	}
	messageType := req.Opts["type"]
	switch messageType {
	case "", "chat":
		messageType = "chat"
	case "groupchat":
	default:
		return nil, fmt.Errorf("invalid xmpp message type `%s`; use chat or groupchat", messageType)
	}
	out := &xmppRequest{jid: jid, messageType: messageType}
	if to := req.Opts["to"]; to != "" {
		if _, err := parseXMPPJID(to); err != nil {
			return nil, fmt.Errorf("invalid xmpp recipient jid")
		}
		out.to = to
	}
	body, err := req.Body.Decode()
	if err != nil {
		return nil, err
	}
	if len(body) > 0 {
		if !utf8.Valid(body) {
			return nil, fmt.Errorf("xmpp body must be valid utf8")
		}
		if out.to == "" {
			return nil, fmt.Errorf("xmpp send requires opts.to")
		}
	}
	out.body = body
	return out, nil
}

func xmppConnect(cfg Config, req *Request) (*xmppSession, *xmppRequest, error) {
	parsed, err := parseXMPPRequest(req)
	if err != nil {
		return nil, nil, err
	}
	if len(parsed.body) > cfg.Limit() {
		return nil, nil, fmt.Errorf("request body exceeds max_bytes (%d)", cfg.Limit())
	}
	host, port := parsed.jid.domain, "5222"
	if req.Target != "" {
		host, port, _, err = parseMessagingTarget(req.Target, "xmpp", "5222", map[string]bool{"xmpp": true})
		if err != nil {
			return nil, nil, err
		}
	}
	conn, err := dialMessaging(cfg, host, port, req.Timeout(cfg))
	if err != nil {
		return nil, nil, redactedError(fmt.Sprintf("xmpp %v", err), req.Opts["password"])
	}
	deadline := time.Now().Add(req.Timeout(cfg))
	session := &xmppSession{
		conn:     conn,
		limit:    cfg.Limit(),
		local:    parsed.jid.local,
		domain:   parsed.jid.domain,
		password: req.Opts["password"],
	}
	session.reset(conn)
	if err := session.startTLS(deadline); err != nil {
		_ = conn.Close()
		return nil, nil, redactedError(err.Error(), session.password)
	}
	features, err := session.authenticate(deadline)
	if err != nil {
		_ = session.conn.Close()
		return nil, nil, redactedError(err.Error(), session.password)
	}
	if err := session.bind(features, deadline); err != nil {
		_ = session.conn.Close()
		return nil, nil, redactedError(err.Error(), session.password)
	}
	_ = session.conn.SetDeadline(time.Time{})
	return session, parsed, nil
}

func (s *xmppSession) send(parsed *xmppRequest, deadline time.Time) error {
	if len(parsed.body) == 0 {
		return nil
	}
	message := `<message to="` + xmppEscape(parsed.to) + `" type="` + parsed.messageType +
		`" id="apidoc-` + strconv.FormatInt(time.Now().UnixNano(), 10) + `"><body>` +
		xmppEscape(string(parsed.body)) + `</body></message>`
	if err := s.write(deadline, message); err != nil {
		return redactErr(fmt.Errorf("xmpp send failed: %v", err), s.password)
	}
	return nil
}

func (s *xmppSession) readMessage(deadline time.Time) (string, string, error) {
	for {
		node, err := s.readElement(deadline)
		if err != nil {
			return "", "", err
		}
		if node.name.Local == "error" && node.name.Space == xmppStreamNS {
			return "", "", fmt.Errorf("xmpp stream error: %s", xmppErrorText(node))
		}
		if node.name.Local != "message" || node.name.Space != xmppClientNS {
			continue
		}
		body := node.child(xmppClientNS, "body")
		if body == nil {
			continue
		}
		return node.attr("from"), body.text.String(), nil
	}
}

func (s *xmppSession) close() {
	_ = s.conn.SetWriteDeadline(time.Now().Add(100 * time.Millisecond))
	_, _ = io.WriteString(s.conn, `</stream:stream>`)
	_ = s.conn.Close()
}

func executeXMPP(cfg Config, req *Request) (*Response, error) {
	started := time.Now()
	session, parsed, err := xmppConnect(cfg, req)
	if err != nil {
		return nil, err
	}
	defer session.close()
	if err := session.send(parsed, time.Now().Add(req.Timeout(cfg))); err != nil {
		return nil, err
	}
	want := wantMessages(req)
	deadline := time.Now().Add(windowDuration(cfg, req))
	messages := []map[string]any{}
	total := 0
	for len(messages) < want && time.Now().Before(deadline) {
		from, body, err := session.readMessage(deadline)
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				break
			}
			return nil, redactedError(fmt.Sprintf("xmpp receive failed: %v", err), session.password)
		}
		if err := addMessageBytes(&total, len(body), cfg.Limit()); err != nil {
			return nil, err
		}
		messages = append(messages, map[string]any{"from": from, "body": body})
	}
	encoded, err := json.Marshal(map[string]any{"messages": messages})
	if err != nil {
		return nil, fmt.Errorf("xmpp response could not be encoded: %v", err)
	}
	return &Response{
		Status:  0,
		MS:      time.Since(started).Milliseconds(),
		Headers: map[string]string{"x-messages": strconv.Itoa(len(messages))},
		Body:    Payload{Encoding: "utf8", Data: string(encoded)},
	}, nil
}

type xmppStream struct {
	session *xmppSession
}

func openXMPPStream(cfg Config, req *Request) (StreamSession, error) {
	session, parsed, err := xmppConnect(cfg, req)
	if err != nil {
		return nil, err
	}
	if err := session.send(parsed, time.Now().Add(req.Timeout(cfg))); err != nil {
		session.close()
		return nil, err
	}
	return &xmppStream{session: session}, nil
}

func (s *xmppStream) ReadFrame() ([]byte, error) {
	for {
		_, body, err := s.session.readMessage(time.Time{})
		if err != nil {
			return nil, redactedError(fmt.Sprintf("xmpp receive failed: %v", err), s.session.password)
		}
		return []byte(body), nil
	}
}

func (s *xmppStream) Close() error {
	s.session.close()
	return nil
}

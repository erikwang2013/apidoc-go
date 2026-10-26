//go:build apidoc_debug_xmpp || apidoc_debug_messaging || apidoc_debug_all

package debug

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"math/big"
	"net"
	"strings"
	"sync"
	"testing"
	"time"
)

const xmppTestHeader = `<?xml version="1.0"?>` +
	`<stream:stream from="localhost" id="test" xmlns="jabber:client"` +
	` xmlns:stream="http://etherx.jabber.org/streams" version="1.0">`

type xmppTestServer struct {
	addr     string
	deliver  []string
	echo     bool
	session  bool
	noTLS    bool
	authFail bool
	authText string
	cert     tls.Certificate

	mu       sync.Mutex
	auth     []byte
	messages []xmppTestMethod
}

type xmppTestMethod struct {
	to   string
	kind string
	body string
}

func xmppTestCertificate(t *testing.T, domain string) (tls.Certificate, *x509.CertPool) {
	t.Helper()
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	caTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "apidoc test ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	caCert, err := x509.ParseCertificate(caDER)
	if err != nil {
		t.Fatal(err)
	}
	serverKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	serverTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: domain},
		DNSNames:     []string{domain},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	serverDER, err := x509.CreateCertificate(rand.Reader, serverTemplate, caCert, &serverKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(caCert)
	cert := tls.Certificate{Certificate: [][]byte{serverDER}, PrivateKey: serverKey}
	return cert, pool
}

func newXMPPTestServer(t *testing.T) *xmppTestServer {
	t.Helper()
	return newXMPPTestServerDomain(t, "localhost")
}

func newXMPPTestServerDomain(t *testing.T, domain string) *xmppTestServer {
	t.Helper()
	cert, pool := xmppTestCertificate(t, domain)
	old := xmppRootCAs
	xmppRootCAs = pool
	t.Cleanup(func() { xmppRootCAs = old })

	ln := listenLocal(t)
	srv := &xmppTestServer{addr: ln.Addr().String(), cert: cert}
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

func xmppTestMessage(from, body string) string {
	return `<message from="` + xmppEscape(from) + `" type="chat"><body>` + xmppEscape(body) + `</body></message>`
}

func (s *xmppTestServer) handle(conn net.Conn) {
	defer conn.Close()
	deadline := time.Now().Add(2 * time.Second)
	srv := &xmppSession{conn: conn, limit: 1 << 20}
	srv.reset(conn)
	if srv.readStreamStart(deadline) != nil {
		return
	}
	if srv.write(deadline, xmppTestHeader+`<stream:features><starttls xmlns="urn:ietf:params:xml:ns:xmpp-tls"><required/></starttls></stream:features>`) != nil {
		return
	}
	if s.noTLS {
		return
	}
	node, err := srv.readElement(deadline)
	if err != nil || node.name.Local != "starttls" {
		return
	}
	if srv.write(deadline, `<proceed xmlns="urn:ietf:params:xml:ns:xmpp-tls"/>`) != nil {
		return
	}
	tlsConn := tls.Server(conn, &tls.Config{Certificates: []tls.Certificate{s.cert}})
	_ = tlsConn.SetDeadline(deadline)
	if err := tlsConn.Handshake(); err != nil {
		return
	}
	_ = tlsConn.SetDeadline(time.Time{})
	srv.conn = tlsConn
	srv.reset(tlsConn)
	if srv.readStreamStart(deadline) != nil {
		return
	}
	if srv.write(deadline, xmppTestHeader+`<stream:features><mechanisms xmlns="urn:ietf:params:xml:ns:xmpp-sasl"><mechanism>PLAIN</mechanism></mechanisms></stream:features>`) != nil {
		return
	}
	auth, err := srv.readElement(deadline)
	if err != nil || auth.name.Local != "auth" {
		return
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(auth.text.String()))
	if err != nil {
		return
	}
	s.mu.Lock()
	s.auth = raw
	s.mu.Unlock()
	if s.authFail {
		text := s.authText
		if text == "" {
			text = "bad credentials"
		}
		_ = srv.write(deadline, `<failure xmlns="urn:ietf:params:xml:ns:xmpp-sasl">`+xmppEscape(text)+`</failure>`)
		return
	}
	if srv.write(deadline, `<success xmlns="urn:ietf:params:xml:ns:xmpp-sasl"/>`) != nil {
		return
	}
	if srv.readStreamStart(deadline) != nil {
		return
	}
	features := `<stream:features><bind xmlns="urn:ietf:params:xml:ns:xmpp-bind"/>`
	if s.session {
		features += `<session xmlns="urn:ietf:params:xml:ns:xmpp-session"/>`
	}
	features += `</stream:features>`
	if srv.write(deadline, xmppTestHeader+features) != nil {
		return
	}
	bind, err := srv.readElement(deadline)
	if err != nil || bind.name.Local != "iq" || bind.attr("id") != "bind-1" {
		return
	}
	if srv.write(deadline, `<iq type="result" id="bind-1"><bind xmlns="urn:ietf:params:xml:ns:xmpp-bind"><jid>user@localhost/res</jid></bind></iq>`) != nil {
		return
	}
	if s.session {
		session, err := srv.readElement(deadline)
		if err != nil || session.attr("id") != "session-1" {
			return
		}
		if srv.write(deadline, `<iq type="result" id="session-1"/>`) != nil {
			return
		}
	}
	for _, body := range s.deliver {
		if srv.write(deadline, xmppTestMessage("peer@localhost/res", body)) != nil {
			return
		}
	}
	for {
		node, err := srv.readElement(deadline)
		if err != nil {
			return
		}
		if node.name.Local != "message" {
			continue
		}
		body := node.child(xmppClientNS, "body")
		if body == nil {
			continue
		}
		message := xmppTestMethod{to: node.attr("to"), kind: node.attr("type"), body: body.text.String()}
		s.mu.Lock()
		s.messages = append(s.messages, message)
		s.mu.Unlock()
		if s.echo {
			if srv.write(deadline, xmppTestMessage("peer@localhost/res", message.body)) != nil {
				return
			}
		}
	}
}

func (s *xmppTestServer) authBytes() []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]byte{}, s.auth...)
}

func (s *xmppTestServer) sent() []xmppTestMethod {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]xmppTestMethod{}, s.messages...)
}

func xmppTestRequest(addr string, opts map[string]string, body *Payload) *Request {
	all := map[string]string{"jid": "user@localhost", "password": "pw-secret"}
	for key, value := range opts {
		all[key] = value
	}
	return &Request{Kind: "xmpp", Target: "xmpp://" + addr, Opts: all, Body: body}
}

func TestXMPPJIDAndEscaping(t *testing.T) {
	jid, err := parseXMPPJID("user@example.com/resource")
	if err != nil || jid.local != "user" || jid.domain != "example.com" || jid.resource != "resource" {
		t.Fatalf("jid = %#v err=%v", jid, err)
	}
	jid, err = parseXMPPJID("user@example.com")
	if err != nil || jid.resource != "" {
		t.Fatalf("jid = %#v err=%v", jid, err)
	}
	for _, bad := range []string{"", "user", "@example.com", "user@", "user@/res", "a b@example.com", "u@@x", strings.Repeat("a", 1024) + "@x"} {
		if _, err := parseXMPPJID(bad); err == nil {
			t.Fatalf("jid %q was accepted", bad)
		}
	}
	if got := xmppEscape(`<a&"'>`); got != `&lt;a&amp;&#34;&#39;&gt;` {
		t.Fatalf("escaped = %q", got)
	}
}

func TestXMPPValidation(t *testing.T) {
	body := &Payload{Encoding: "utf8", Data: "hello"}
	cases := map[string]*Request{
		"missing jid":      {Kind: "xmpp", Target: "xmpp://127.0.0.1:1", Opts: map[string]string{"password": "pw"}},
		"missing password": {Kind: "xmpp", Target: "xmpp://127.0.0.1:1", Opts: map[string]string{"jid": "user@localhost"}},
		"invalid type":     {Kind: "xmpp", Target: "xmpp://127.0.0.1:1", Opts: map[string]string{"jid": "user@localhost", "password": "pw", "type": "headline"}},
		"body without to":  {Kind: "xmpp", Target: "xmpp://127.0.0.1:1", Opts: map[string]string{"jid": "user@localhost", "password": "pw"}, Body: body},
		"invalid to":       {Kind: "xmpp", Target: "xmpp://127.0.0.1:1", Opts: map[string]string{"jid": "user@localhost", "password": "pw", "to": "nobody"}, Body: body},
		"invalid utf8":     {Kind: "xmpp", Target: "xmpp://127.0.0.1:1", Opts: map[string]string{"jid": "user@localhost", "password": "pw", "to": "peer@localhost"}, Body: &Payload{Encoding: "hex", Data: "ff"}},
		"invalid scheme":   {Kind: "xmpp", Target: "xmpps://127.0.0.1:1", Opts: map[string]string{"jid": "user@localhost", "password": "pw"}},
	}
	for name, req := range cases {
		if _, err := Execute(messagingConfig(), req); err == nil {
			t.Fatalf("%s was accepted", name)
		}
	}
}

func TestXMPPSendReceiveAndSession(t *testing.T) {
	srv := newXMPPTestServer(t)
	srv.echo = true
	srv.session = true
	resp, err := Execute(messagingConfig(), xmppTestRequest(srv.addr, map[string]string{"to": "peer@localhost"}, &Payload{Encoding: "utf8", Data: "hello <world>"}))
	if err != nil {
		t.Fatal(err)
	}
	if auth := string(srv.authBytes()); auth != "\x00user\x00pw-secret" {
		t.Fatalf("auth = %q", auth)
	}
	sent := srv.sent()
	if len(sent) != 1 || sent[0].to != "peer@localhost" || sent[0].kind != "chat" || sent[0].body != "hello <world>" {
		t.Fatalf("sent = %#v", sent)
	}
	messages := decodeMessagingJSON(t, resp)["messages"].([]any)
	if len(messages) != 1 {
		t.Fatalf("messages = %#v", messages)
	}
	message := messages[0].(map[string]any)
	if message["from"] != "peer@localhost/res" || message["body"] != "hello <world>" {
		t.Fatalf("message = %#v", message)
	}
	if resp.Headers["x-messages"] != "1" {
		t.Fatalf("headers = %#v", resp.Headers)
	}
}

func TestXMPPDeliverWindowStreamAndCaps(t *testing.T) {
	deliver := newXMPPTestServer(t)
	deliver.deliver = []string{"one", "two"}
	resp, err := Execute(messagingConfig(), xmppTestRequest(deliver.addr, map[string]string{"messages": "2", "duration_ms": "500"}, nil))
	if err != nil {
		t.Fatal(err)
	}
	messages := decodeMessagingJSON(t, resp)["messages"].([]any)
	if len(messages) != 2 || messages[1].(map[string]any)["body"] != "two" {
		t.Fatalf("messages = %#v", messages)
	}

	empty := newXMPPTestServer(t)
	started := time.Now()
	resp, err = Execute(messagingConfig(), xmppTestRequest(empty.addr, map[string]string{"duration_ms": "40"}, nil))
	if err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("empty window took %v", elapsed)
	}
	if messages := decodeMessagingJSON(t, resp)["messages"].([]any); len(messages) != 0 {
		t.Fatalf("empty messages = %#v", messages)
	}

	big := newXMPPTestServer(t)
	big.deliver = []string{strings.Repeat("x", 100)}
	cfg := messagingConfig()
	cfg.MaxBytes = 64
	if _, err := Execute(cfg, xmppTestRequest(big.addr, nil, nil)); err == nil || !strings.Contains(err.Error(), "max_bytes") {
		t.Fatalf("cap err = %v", err)
	}
	if _, err := Execute(cfg, xmppTestRequest(big.addr, map[string]string{"to": "peer@localhost"}, &Payload{Encoding: "utf8", Data: strings.Repeat("x", 100)})); err == nil || !strings.Contains(err.Error(), "max_bytes") {
		t.Fatalf("request cap err = %v", err)
	}

	streaming := newXMPPTestServer(t)
	streaming.deliver = []string{"one", "two"}
	stream, err := OpenStream(messagingConfig(), xmppTestRequest(streaming.addr, nil, nil))
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

func TestXMPPCertificateVerification(t *testing.T) {
	srv := newXMPPTestServerDomain(t, "wrong.example")
	_, err := Execute(messagingConfig(), xmppTestRequest(srv.addr, nil, nil))
	if err == nil || !strings.Contains(err.Error(), "STARTTLS handshake failed") {
		t.Fatalf("certificate err = %v", err)
	}
	if strings.Contains(err.Error(), "pw-secret") {
		t.Fatalf("certificate error leaked the password: %v", err)
	}
}

func TestXMPPDepthCap(t *testing.T) {
	client, server := net.Pipe()
	session := &xmppSession{conn: client, limit: 1 << 20}
	session.reset(client)
	deep := strings.Repeat("<a>", xmppMaxDepth+1) + strings.Repeat("</a>", xmppMaxDepth+1)
	done := make(chan struct{})
	go func() {
		_, _ = server.Write([]byte(deep))
		close(done)
	}()
	_, err := session.readElement(time.Now().Add(time.Second))
	_ = server.Close()
	_ = client.Close()
	<-done
	if err == nil || !strings.Contains(err.Error(), "nesting") {
		t.Fatalf("depth err = %v", err)
	}
}

func TestXMPPAuthFailureNoTLSAndSSRF(t *testing.T) {
	failing := newXMPPTestServer(t)
	failing.authFail = true
	failing.authText = "bad password pw-secret"
	_, err := Execute(messagingConfig(), xmppTestRequest(failing.addr, nil, nil))
	if err == nil || !strings.Contains(err.Error(), "SASL authentication failed") {
		t.Fatalf("auth err = %v", err)
	}
	if strings.Contains(err.Error(), "pw-secret") || !strings.Contains(err.Error(), "***") {
		t.Fatalf("auth error leaked the password: %v", err)
	}

	plain := newXMPPTestServer(t)
	plain.noTLS = true
	if _, err := Execute(messagingConfig(), xmppTestRequest(plain.addr, nil, nil)); err == nil || !strings.Contains(err.Error(), "STARTTLS") {
		t.Fatalf("starttls err = %v", err)
	}

	private := Config{EnableServerSide: true, TimeoutMS: 500}
	if _, err := Execute(private, xmppTestRequest(failing.addr, nil, nil)); err == nil || !strings.Contains(err.Error(), "private") {
		t.Fatalf("ssrf err = %v", err)
	}
}

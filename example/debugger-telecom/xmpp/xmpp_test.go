package xmpp

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/xml"
	"math/big"
	"net"
	"strings"
	"testing"
	"time"

	"mellium.im/sasl"
	"mellium.im/xmlstream"
	mxmpp "mellium.im/xmpp"
	"mellium.im/xmpp/jid"
	"mellium.im/xmpp/stanza"

	"github.com/erikwang2013/apidoc-go"
)

func selfSigned(t *testing.T) tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "localhost"},
		DNSNames:              []string{"localhost"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
}

// startServer runs a minimal XMPP server: STARTTLS, SASL PLAIN, bind, and an
// echo bot that answers every message with "echo: <body>".
func startServer(t *testing.T) string {
	t.Helper()
	cert := selfSigned(t)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go serveConn(conn, cert)
		}
	}()
	_, port, _ := net.SplitHostPort(ln.Addr().String())
	return "localhost:" + port
}

func serveConn(conn net.Conn, cert tls.Certificate) {
	defer conn.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	sess, err := mxmpp.ReceiveClientSession(ctx, jid.MustParse("localhost"), conn,
		mxmpp.StartTLS(&tls.Config{Certificates: []tls.Certificate{cert}}),
		mxmpp.SASLServer(func(*sasl.Negotiator) bool { return true }, sasl.Plain),
		mxmpp.BindResource(),
	)
	if err != nil {
		return
	}
	_ = sess.Serve(mxmpp.HandlerFunc(func(tr xmlstream.TokenReadEncoder, start *xml.StartElement) error {
		inner, err := xmlstream.ReadAll(xmlstream.Inner(tr))
		if err != nil {
			return err
		}
		if start.Name.Local != "message" {
			return nil
		}
		var body strings.Builder
		for _, tok := range inner {
			if cd, ok := tok.(xml.CharData); ok {
				body.Write(cd)
			}
		}
		reply := stanza.Message{Type: stanza.ChatMessage}.Wrap(
			xmlstream.Wrap(xmlstream.Token(xml.CharData("echo: "+body.String())),
				xml.StartElement{Name: xml.Name{Local: "body"}}),
		)
		_, err = xmlstream.Copy(tr, reply)
		return err
	}))
}

func waitEvent(t *testing.T, ch <-chan apidoc.DebugEvent, match func(apidoc.DebugEvent) bool) apidoc.DebugEvent {
	t.Helper()
	var seen []apidoc.DebugEvent
	timeout := time.After(3 * time.Second)
	for {
		select {
		case ev, ok := <-ch:
			if !ok {
				t.Fatalf("events closed before match: %+v", seen)
			}
			seen = append(seen, ev)
			if match(ev) {
				return ev
			}
		case <-timeout:
			t.Fatalf("timed out waiting for event: %+v", seen)
		}
	}
}

func TestSessionSendReceivesEcho(t *testing.T) {
	addr := startServer(t)
	sess, err := New().Open(context.Background(), apidoc.DebugRequest{
		URL: "xmpp://alice@" + addr,
		Options: map[string]string{
			"password":   "secret",
			"tls":        "insecure",
			"timeout_ms": "5000",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()

	waitEvent(t, sess.Events(), func(ev apidoc.DebugEvent) bool { return ev.Name == "READY" })

	if err := sess.Send(context.Background(), []byte("hello"), map[string]string{"to": "bob@localhost"}); err != nil {
		t.Fatal(err)
	}
	ev := waitEvent(t, sess.Events(), func(ev apidoc.DebugEvent) bool { return ev.Direction == "in" && ev.Name == "message" })
	if !strings.Contains(ev.Data, "echo: hello") {
		t.Fatalf("reply = %q", ev.Data)
	}
}

func TestExecMessage(t *testing.T) {
	addr := startServer(t)
	res, err := New().Exec(context.Background(), apidoc.DebugRequest{
		Method: "MESSAGE",
		URL:    "xmpp://alice@" + addr,
		Body:   []byte("ping"),
		Options: map[string]string{
			"password":   "secret",
			"tls":        "insecure",
			"to":         "bob@localhost",
			"reply_ms":   "3000",
			"timeout_ms": "5000",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != "OK" {
		t.Fatalf("status = %q", res.Status)
	}
	var found bool
	for _, ev := range res.Events {
		if ev.Direction == "in" && strings.Contains(ev.Data, "echo: ping") {
			found = true
		}
	}
	if !found {
		t.Fatalf("no echo event: %+v", res.Events)
	}
}

func TestBadInput(t *testing.T) {
	if _, err := New().Open(context.Background(), apidoc.DebugRequest{URL: "http://localhost"}); err == nil {
		t.Fatal("expected scheme error")
	}
	if _, err := New().Open(context.Background(), apidoc.DebugRequest{URL: "xmpp://alice@localhost:1"}); err == nil {
		t.Fatal("expected missing password error")
	}
	s := &session{}
	if err := s.Send(context.Background(), nil, nil); err == nil {
		t.Fatal("expected empty message error")
	}
}

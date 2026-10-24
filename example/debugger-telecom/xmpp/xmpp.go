// Package xmpp is an apidoc streaming debug adapter for XMPP client
// sessions. It negotiates STARTTLS, SASL PLAIN and resource binding with
// mellium, then streams incoming stanzas as transcript events. Send wraps a
// text body into a message stanza, or forwards raw XML when meta["raw"] is set.
package xmpp

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/xml"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"mellium.im/sasl"
	"mellium.im/xmlstream"
	mxmpp "mellium.im/xmpp"
	"mellium.im/xmpp/jid"
	"mellium.im/xmpp/stanza"

	"github.com/erikwang2013/apidoc-go"
)

type debugger struct{}

// New returns the XMPP debugger.
func New() apidoc.StreamDebugger { return debugger{} }

func (debugger) Protocol() string { return "xmpp" }

func (debugger) Spec() apidoc.DebugSpec {
	return apidoc.DebugSpec{
		Mode:         "server",
		Actions:      []string{"MESSAGE"},
		Encodings:    []string{"text", "xml"},
		ResponseView: "transcript",
		Streaming:    true,
		ConnectFields: []apidoc.DebugField{
			{Name: "username", Label: "JID localpart (default: URL user)", Type: "text"},
			{Name: "password", Label: "Password (SASL PLAIN)", Type: "password", Secret: true, Required: true},
			{Name: "tls", Label: "TLS verification", Type: "select", Options: []string{"verify", "insecure"}, Default: "verify"},
			{Name: "timeout_ms", Label: "Connect timeout (ms)", Type: "number", Default: "5000"},
		},
	}
}

func (d debugger) Exec(ctx context.Context, req apidoc.DebugRequest) (*apidoc.DebugResponse, error) {
	sess, err := d.Open(ctx, req)
	if err != nil {
		return nil, err
	}
	start := time.Now()
	if len(req.Body) > 0 {
		if err := sess.Send(ctx, req.Body, req.Options); err != nil {
			_ = sess.Close()
			return nil, err
		}
	}
	wait := 500 * time.Millisecond
	if ms, err := strconv.Atoi(req.Options["reply_ms"]); err == nil && ms >= 0 {
		wait = time.Duration(ms) * time.Millisecond
	}
	res := &apidoc.DebugResponse{Status: "SENT"}
	timer := time.NewTimer(wait)
	defer timer.Stop()
loop:
	for {
		select {
		case ev, ok := <-sess.Events():
			if !ok {
				break loop
			}
			res.Events = append(res.Events, ev)
			if ev.Direction == "in" && ev.Name != "READY" {
				res.Status = "OK"
				break loop
			}
		case <-timer.C:
			break loop
		case <-ctx.Done():
			_ = sess.Close()
			return nil, ctx.Err()
		}
	}
	_ = sess.Close()
	for ev := range sess.Events() {
		res.Events = append(res.Events, ev)
	}
	res.Duration = time.Since(start)
	return res, nil
}

func (debugger) Open(ctx context.Context, req apidoc.DebugRequest) (apidoc.Session, error) {
	u, err := url.Parse(req.URL)
	if err != nil {
		return nil, fmt.Errorf("xmpp: bad url: %w", err)
	}
	if u.Scheme != "xmpp" {
		return nil, fmt.Errorf("xmpp: unsupported scheme %q", u.Scheme)
	}
	if u.Host == "" {
		return nil, fmt.Errorf("xmpp: url has no host")
	}
	domain := u.Hostname()
	username := opt(req.Options, "username", "")
	if username == "" && u.User != nil {
		username = u.User.Username()
	}
	if username == "" {
		return nil, fmt.Errorf("xmpp: username is required (URL user or username option)")
	}
	password := req.Options["password"]
	if password == "" {
		return nil, fmt.Errorf("xmpp: password is required")
	}
	resource := strings.TrimPrefix(u.Path, "/")
	origin, err := jid.New(username, domain, resource)
	if err != nil {
		return nil, fmt.Errorf("xmpp: bad jid: %w", err)
	}
	hostPort := u.Host
	if u.Port() == "" {
		hostPort = net.JoinHostPort(domain, "5222")
	}
	timeout := 5 * time.Second
	if ms, err := strconv.Atoi(req.Options["timeout_ms"]); err == nil && ms > 0 {
		timeout = time.Duration(ms) * time.Millisecond
	}

	conn, err := (&net.Dialer{Timeout: timeout}).DialContext(ctx, "tcp", hostPort)
	if err != nil {
		return nil, fmt.Errorf("xmpp: dial %s: %w", hostPort, err)
	}
	tlsCfg := &tls.Config{ServerName: domain, MinVersion: tls.VersionTLS12}
	if req.Options["tls"] == "insecure" {
		tlsCfg.InsecureSkipVerify = true
	}

	s := &session{events: make(chan apidoc.DebugEvent, 256), done: make(chan struct{})}
	sess, err := mxmpp.NewClientSession(ctx, origin, conn,
		mxmpp.StartTLS(tlsCfg),
		mxmpp.SASL("", password, sasl.Plain),
		mxmpp.BindResource(),
	)
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("xmpp: negotiate: %w", err)
	}
	s.sess = sess
	s.emit("in", "READY", sess.LocalAddr().String(), "text")
	go func() {
		defer close(s.done)
		defer close(s.events)
		if err := sess.Serve(&handler{s: s}); err != nil {
			s.emit("in", "CLOSE", err.Error(), "text")
		}
	}()
	return s, nil
}

type session struct {
	sess   *mxmpp.Session
	events chan apidoc.DebugEvent
	done   chan struct{}
	once   sync.Once
}

type handler struct {
	s *session
}

func (h *handler) HandleXMPP(t xmlstream.TokenReadEncoder, start *xml.StartElement) error {
	// The session handler wraps the reader in InnerElement, which still yields
	// the stanza's own end tag; Inner drops it.
	inner, err := xmlstream.ReadAll(xmlstream.Inner(t))
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	enc := xml.NewEncoder(&buf)
	if err := enc.EncodeToken(*start); err != nil {
		return err
	}
	for _, tok := range inner {
		if err := enc.EncodeToken(tok); err != nil {
			return err
		}
	}
	if err := enc.EncodeToken(start.End()); err != nil {
		return err
	}
	if err := enc.Flush(); err != nil {
		return err
	}
	h.s.emit("in", start.Name.Local, buf.String(), "xml")
	return nil
}

func (s *session) Send(ctx context.Context, msg []byte, meta map[string]string) error {
	if len(msg) == 0 {
		return fmt.Errorf("xmpp: empty message")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	raw := meta["raw"]
	if raw == "1" || raw == "true" || meta["stanza"] == "raw" {
		if err := s.sess.Send(ctx, xml.NewDecoder(bytes.NewReader(msg))); err != nil {
			return err
		}
		s.emit("out", "RAW", string(msg), "xml")
		return nil
	}
	target := meta["to"]
	if target == "" {
		return fmt.Errorf("xmpp: meta.to is required for message send")
	}
	to, err := jid.Parse(target)
	if err != nil {
		return fmt.Errorf("xmpp: bad to jid: %w", err)
	}
	body := xmlstream.Wrap(
		xmlstream.Token(xml.CharData(msg)),
		xml.StartElement{Name: xml.Name{Local: "body"}},
	)
	if err := s.sess.Send(ctx, stanza.Message{To: to, Type: stanza.ChatMessage}.Wrap(body)); err != nil {
		return err
	}
	s.emit("out", "MESSAGE", "to="+target+" "+string(msg), "text")
	return nil
}

func (s *session) Events() <-chan apidoc.DebugEvent { return s.events }

func (s *session) Close() error {
	// Close only writes </stream:stream>; close the socket too so Serve returns.
	err := s.sess.Close()
	_ = s.sess.Conn().Close()
	s.once.Do(func() { <-s.done })
	return err
}

func (s *session) emit(dir, name, data, encoding string) {
	select {
	case s.events <- apidoc.DebugEvent{Time: time.Now(), Direction: dir, Name: name, Data: data, Encoding: encoding}:
	default:
	}
}

func opt(opts map[string]string, key, def string) string {
	if v := opts[key]; v != "" {
		return v
	}
	return def
}

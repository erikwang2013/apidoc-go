// Package sip is an apidoc debug adapter for SIP signalling over UDP.
// Exec sends OPTIONS, or performs a minimal INVITE / ACK / BYE dialog.
// sipgo builds and parses the messages; the transcript contains the full
// request and response text.
package sip

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/emiago/sipgo"
	"github.com/emiago/sipgo/sip"

	"github.com/erikwang2013/apidoc-go"
)

type debugger struct{}

// New returns the SIP debugger.
func New() apidoc.Debugger { return debugger{} }

func (debugger) Protocol() string { return "sip" }

func (debugger) Spec() apidoc.DebugSpec {
	return apidoc.DebugSpec{
		Mode:         "server",
		Actions:      []string{"OPTIONS", "INVITE"},
		Encodings:    []string{"text"},
		ResponseView: "transcript",
		ConnectFields: []apidoc.DebugField{
			{Name: "username", Label: "Caller / auth user", Type: "text", Default: "apidoc"},
			{Name: "password", Label: "Password (digest auth)", Type: "password", Secret: true},
			{Name: "local_host", Label: "Local host", Type: "text", Default: "127.0.0.1"},
			{Name: "timeout_ms", Label: "Timeout (ms)", Type: "number", Default: "5000"},
		},
	}
}

func (debugger) Exec(ctx context.Context, req apidoc.DebugRequest) (*apidoc.DebugResponse, error) {
	method := strings.ToUpper(req.Method)
	if method == "" {
		method = "OPTIONS"
	}
	if method != "OPTIONS" && method != "INVITE" {
		return nil, fmt.Errorf("sip: unsupported method %q", req.Method)
	}
	recipient, err := parseTarget(req.URL)
	if err != nil {
		return nil, err
	}
	username := opt(req.Options, "username", "apidoc")
	localHost := opt(req.Options, "local_host", "127.0.0.1")
	timeout := 5 * time.Second
	if ms, err := strconv.Atoi(req.Options["timeout_ms"]); err == nil && ms > 0 {
		timeout = time.Duration(ms) * time.Millisecond
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ua, err := sipgo.NewUA(
		sipgo.WithUserAgent(username),
		sipgo.WithUserAgentHostname(localHost),
	)
	if err != nil {
		return nil, err
	}
	defer ua.Close()

	// Reserve a UDP port so the Contact header in INVITE resolves back to us.
	localAddr, err := reserveUDP(localHost)
	if err != nil {
		return nil, err
	}
	client, err := sipgo.NewClient(ua,
		sipgo.WithClientAddr(localAddr),
		sipgo.WithClientNAT(),
	)
	if err != nil {
		return nil, err
	}

	start := time.Now()
	res := &apidoc.DebugResponse{Status: "OK", Encoding: "text"}
	var events []apidoc.DebugEvent

	if method == "OPTIONS" {
		r := sip.NewRequest(sip.OPTIONS, recipient)
		r.SetTransport("UDP")
		resp, err := client.Do(ctx, r)
		if err != nil {
			return nil, err
		}
		events = append(events,
			textEvent("out", "OPTIONS", r.String()),
			textEvent("in", "SIP "+strconv.Itoa(resp.StatusCode), resp.String()),
		)
		res.Status = statusOf(resp)
		res.Body = resp.Body()
		res.Headers = headersOf(resp)
	} else {
		events, res.Status, res.Headers, err = invite(ctx, client, recipient, req, username, localAddr)
		if err != nil {
			return nil, err
		}
	}
	res.Events = events
	res.Duration = time.Since(start)
	return res, nil
}

func invite(ctx context.Context, client *sipgo.Client, recipient sip.Uri, req apidoc.DebugRequest, username, localAddr string) ([]apidoc.DebugEvent, string, map[string]string, error) {
	host, portStr, err := net.SplitHostPort(localAddr)
	if err != nil {
		return nil, "", nil, err
	}
	port, _ := strconv.Atoi(portStr)
	contact := sip.ContactHeader{Address: sip.Uri{Scheme: "sip", User: username, Host: host, Port: port}}
	dc := sipgo.NewDialogClientCache(client, contact)

	body := req.Body
	if len(body) == 0 {
		body = []byte(defaultSDP(host))
	}
	contentType := sip.ContentTypeHeader("application/sdp")
	var events []apidoc.DebugEvent

	dialog, err := dc.Invite(ctx, recipient, body, &contentType)
	if err != nil {
		return nil, "", nil, err
	}
	defer dialog.Close()
	events = append(events, textEvent("out", "INVITE", dialog.InviteRequest.String()))

	answer := sipgo.AnswerOptions{
		Username: opt(req.Options, "username", "apidoc"),
		Password: req.Options["password"],
		OnResponse: func(r *sip.Response) error {
			events = append(events, textEvent("in", "SIP "+strconv.Itoa(r.StatusCode), r.String()))
			return nil
		},
	}
	if err := dialog.WaitAnswer(ctx, answer); err != nil {
		return nil, "", nil, err
	}
	if err := dialog.Ack(ctx); err != nil {
		return nil, "", nil, err
	}
	events = append(events, textEvent("out", "ACK", "ACK"))
	if err := dialog.Bye(ctx); err != nil {
		return nil, "", nil, err
	}
	events = append(events, textEvent("out", "BYE", "BYE"))

	resp := dialog.InviteResponse
	return events, statusOf(resp), headersOf(resp), nil
}

func parseTarget(raw string) (sip.Uri, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return sip.Uri{}, fmt.Errorf("sip: bad url: %w", err)
	}
	if u.Scheme != "sip" && u.Scheme != "sips" {
		return sip.Uri{}, fmt.Errorf("sip: unsupported scheme %q", u.Scheme)
	}
	if u.Host == "" {
		return sip.Uri{}, fmt.Errorf("sip: url needs the form sip://user@host:port")
	}
	recipient := sip.Uri{Scheme: u.Scheme, Host: u.Hostname()}
	if u.User != nil {
		recipient.User = u.User.Username()
	}
	if p := u.Port(); p != "" {
		port, err := strconv.Atoi(p)
		if err != nil {
			return sip.Uri{}, fmt.Errorf("sip: bad port %q", p)
		}
		recipient.Port = port
	}
	params := sip.NewParams()
	params.Add("transport", "udp")
	recipient.UriParams = params
	return recipient, nil
}

// reserveUDP picks a free local port for the SIP client to bind. There is a
// small race between closing this socket and sipgo binding it.
// shortcut: fixed reserved port, upgrade to a custom listener if it collides.
func reserveUDP(host string) (string, error) {
	pc, err := net.ListenPacket("udp", net.JoinHostPort(host, "0"))
	if err != nil {
		return "", err
	}
	addr := pc.LocalAddr().String()
	_ = pc.Close()
	return addr, nil
}

func defaultSDP(host string) string {
	return "v=0\r\no=- 0 0 IN IP4 " + host + "\r\ns=apidoc\r\nc=IN IP4 " + host +
		"\r\nt=0 0\r\nm=audio 9 RTP/AVP 0\r\na=rtpmap:0 PCMU/8000\r\n"
}

func statusOf(resp *sip.Response) string {
	if resp == nil {
		return ""
	}
	return fmt.Sprintf("%d %s", resp.StatusCode, resp.Reason)
}

func headersOf(msg interface{ CloneHeaders() []sip.Header }) map[string]string {
	out := map[string]string{}
	for _, h := range msg.CloneHeaders() {
		if h == nil {
			continue
		}
		name := h.Name()
		if prev, ok := out[name]; ok {
			out[name] = prev + ", " + h.Value()
		} else {
			out[name] = h.Value()
		}
	}
	return out
}

func textEvent(dir, name, data string) apidoc.DebugEvent {
	return apidoc.DebugEvent{Time: time.Now(), Direction: dir, Name: name, Data: data, Encoding: "text"}
}

func opt(opts map[string]string, key, def string) string {
	if v := opts[key]; v != "" {
		return v
	}
	return def
}

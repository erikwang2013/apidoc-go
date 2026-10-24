// Package udp debugs raw UDP endpoints with one datagram in and one out.
package udp

import (
	"context"
	"errors"
	"net"
	"net/url"
	"time"

	"github.com/erikwang2013/apidoc-go"
	"github.com/erikwang2013/apidoc-go/example/debugger-industrial/internal/opts"
)

// New returns the UDP debugger.
func New() apidoc.Debugger {
	return apidoc.DebuggerFunc("udp", apidoc.DebugSpec{
		Mode:         "server",
		Actions:      []string{"SEND"},
		Encodings:    []string{"text", "hex", "base64"},
		ResponseView: "hex",
		ConnectFields: []apidoc.DebugField{
			{Name: "timeout_ms", Label: "Read timeout (ms)", Type: "number", Default: "500"},
		},
	}, exec)
}

func exec(ctx context.Context, req apidoc.DebugRequest) (*apidoc.DebugResponse, error) {
	u, err := url.Parse(req.URL)
	if err != nil || u.Scheme != "udp" || u.Host == "" {
		return nil, errors.New("udp: url must be udp://host:port")
	}
	timeout, err := opts.Timeout(req.Options, "timeout_ms", 500*time.Millisecond, 5*time.Second)
	if err != nil {
		return nil, err
	}
	d := net.Dialer{Timeout: 2 * time.Second}
	conn, err := d.DialContext(ctx, "udp", u.Host)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	_ = conn.SetDeadline(time.Now().Add(timeout))
	if _, err := conn.Write(req.Body); err != nil {
		return nil, err
	}
	buf := make([]byte, 65535)
	n, err := conn.Read(buf)
	if err != nil {
		var ne net.Error
		if errors.As(err, &ne) && ne.Timeout() {
			return &apidoc.DebugResponse{Status: "TIMEOUT", Encoding: req.Encoding,
				Headers: map[string]string{"remote": u.Host}}, nil
		}
		return nil, err
	}
	return &apidoc.DebugResponse{
		Status:   "OK",
		Body:     buf[:n],
		Encoding: req.Encoding,
		Headers:  map[string]string{"remote": u.Host},
	}, nil
}

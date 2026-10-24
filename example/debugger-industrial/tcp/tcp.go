// Package tcp debugs raw TCP endpoints: it writes the payload and returns
// whatever arrives before the read deadline.
package tcp

import (
	"context"
	"errors"
	"io"
	"net"
	"net/url"
	"time"

	"github.com/erikwang2013/apidoc-go"
	"github.com/erikwang2013/apidoc-go/example/debugger-industrial/internal/opts"
)

// New returns the TCP debugger.
func New() apidoc.Debugger {
	return apidoc.DebuggerFunc("tcp", apidoc.DebugSpec{
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
	if err != nil || u.Scheme != "tcp" || u.Host == "" {
		return nil, errors.New("tcp: url must be tcp://host:port")
	}
	timeout, err := opts.Timeout(req.Options, "timeout_ms", 500*time.Millisecond, 5*time.Second)
	if err != nil {
		return nil, err
	}
	d := net.Dialer{Timeout: 2 * time.Second}
	conn, err := d.DialContext(ctx, "tcp", u.Host)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	_ = conn.SetDeadline(time.Now().Add(timeout))
	if _, err := conn.Write(req.Body); err != nil {
		return nil, err
	}
	buf, err := io.ReadAll(conn)
	if err != nil && !errors.Is(err, net.ErrClosed) && !isTimeout(err) {
		return nil, err
	}
	return &apidoc.DebugResponse{
		Status:   "OK",
		Body:     buf,
		Encoding: req.Encoding,
		Headers:  map[string]string{"remote": u.Host},
	}, nil
}

func isTimeout(err error) bool {
	var ne net.Error
	return errors.As(err, &ne) && ne.Timeout()
}

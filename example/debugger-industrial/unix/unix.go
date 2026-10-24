// Package unix debugs Unix domain stream sockets.
package unix

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

// New returns the Unix domain socket debugger.
func New() apidoc.Debugger {
	return apidoc.DebuggerFunc("unix", apidoc.DebugSpec{
		Mode:         "server",
		Actions:      []string{"SEND"},
		Encodings:    []string{"text", "hex", "base64"},
		ResponseView: "hex",
		ConnectFields: []apidoc.DebugField{
			{Name: "timeout_ms", Label: "Read timeout (ms)", Type: "number", Default: "500"},
		},
	}, exec)
}

func socketPath(u *url.URL) string {
	if u.Host != "" {
		return u.Host + u.Path
	}
	return u.Path
}

func exec(ctx context.Context, req apidoc.DebugRequest) (*apidoc.DebugResponse, error) {
	u, err := url.Parse(req.URL)
	if err != nil || u.Scheme != "unix" || socketPath(u) == "" {
		return nil, errors.New("unix: url must be unix:///path/to.sock")
	}
	timeout, err := opts.Timeout(req.Options, "timeout_ms", 500*time.Millisecond, 5*time.Second)
	if err != nil {
		return nil, err
	}
	d := net.Dialer{Timeout: 2 * time.Second}
	conn, err := d.DialContext(ctx, "unix", socketPath(u))
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	_ = conn.SetDeadline(time.Now().Add(timeout))
	if _, err := conn.Write(req.Body); err != nil {
		return nil, err
	}
	buf, err := io.ReadAll(conn)
	if err != nil {
		var ne net.Error
		if !errors.As(err, &ne) || !ne.Timeout() {
			return nil, err
		}
	}
	return &apidoc.DebugResponse{
		Status:   "OK",
		Body:     buf,
		Encoding: req.Encoding,
		Headers:  map[string]string{"socket": socketPath(u)},
	}, nil
}

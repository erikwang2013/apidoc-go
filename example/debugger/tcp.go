// Package debugger contains example debug adapters. They wrap plain
// standard-library clients; apidoc-go itself does not implement protocols.
package debugger

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"strconv"
	"time"

	"github.com/erikwang2013/apidoc-go"
)

// TCP returns a debugger that sends raw bytes to a TCP echo endpoint and
// returns everything received before the read deadline.
func TCP() apidoc.Debugger {
	return apidoc.DebuggerFunc("tcp", apidoc.DebugSpec{
		Mode:         "server",
		Actions:      []string{"SEND"},
		Encodings:    []string{"text", "hex", "base64"},
		BodyIn:       "payload",
		ResponseView: "text",
		ConnectFields: []apidoc.DebugField{
			{Name: "timeout_ms", Label: "Read timeout (ms)", Type: "number", Default: "500"},
		},
	}, func(ctx context.Context, req apidoc.DebugRequest) (*apidoc.DebugResponse, error) {
		u, err := url.Parse(req.URL)
		if err != nil || u.Scheme != "tcp" || u.Host == "" {
			return nil, fmt.Errorf("tcp: bad url %q", req.URL)
		}
		d := net.Dialer{Timeout: 2 * time.Second}
		conn, err := d.DialContext(ctx, "tcp", u.Host)
		if err != nil {
			return nil, err
		}
		defer conn.Close()

		timeout := 500 * time.Millisecond
		if v := req.Options["timeout_ms"]; v != "" {
			if ms, err := strconv.Atoi(v); err == nil && ms > 0 {
				timeout = time.Duration(ms) * time.Millisecond
			}
		}
		if timeout > 5*time.Second {
			timeout = 5 * time.Second
		}
		_ = conn.SetDeadline(time.Now().Add(timeout))
		if _, err := conn.Write(req.Body); err != nil {
			return nil, err
		}
		buf, err := io.ReadAll(conn)
		if err != nil && !errors.Is(err, os.ErrDeadlineExceeded) {
			return nil, err
		}
		return &apidoc.DebugResponse{
			Status: "OK", Body: buf, Encoding: req.Encoding,
			Headers: map[string]string{"remote": u.Host},
		}, nil
	})
}

//go:build apidoc_debug_http3 || apidoc_debug_all

package debug

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
)

func init() { RegisterExecutor("http3", executeHTTP3) }

func executeHTTP3(cfg Config, req *Request) (*Response, error) {
	target, err := CheckTarget(cfg, req.Target)
	if err != nil {
		return nil, err
	}
	if !strings.EqualFold(target.URL.Scheme, "https") {
		return nil, fmt.Errorf("http3 target must use https:// (got `%s`)", target.URL.Scheme)
	}
	method := strings.TrimSpace(req.Method)
	if method == "" {
		method = http.MethodGet
	}
	if strings.EqualFold(method, http.MethodConnect) {
		return nil, fmt.Errorf("HTTP CONNECT is not allowed in server-side debug")
	}
	body, err := req.Body.Decode()
	if err != nil {
		return nil, err
	}
	if len(body) > cfg.Limit() {
		return nil, fmt.Errorf("request body exceeds max_bytes (%d)", cfg.Limit())
	}

	timeout := req.Timeout(cfg)
	transport := &http3.Transport{
		TLSClientConfig: &tls.Config{ServerName: target.Host, MinVersion: tls.VersionTLS13},
		QUICConfig:      &quic.Config{HandshakeIdleTimeout: timeout, MaxIdleTimeout: timeout},
		// Dial only the addresses that already passed the SSRF policy.
		Dial: func(ctx context.Context, _ string, tlsCfg *tls.Config, quicCfg *quic.Config) (quic.EarlyConnection, error) {
			var lastErr error
			for _, ip := range target.Addrs {
				conn, err := quic.DialAddrEarly(ctx, net.JoinHostPort(ip.String(), target.Port), tlsCfg, quicCfg)
				if err == nil {
					return conn, nil
				}
				lastErr = err
			}
			if lastErr == nil {
				lastErr = fmt.Errorf("target resolved to no addresses")
			}
			return nil, lastErr
		},
	}
	defer transport.Close()
	client := &http.Client{
		Transport: transport,
		Timeout:   timeout,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	var reader io.Reader = http.NoBody
	if len(body) > 0 {
		reader = bytes.NewReader(body)
	}
	httpReq, err := http.NewRequestWithContext(ctx, method, target.URL.String(), reader)
	if err != nil {
		return nil, fmt.Errorf("invalid HTTP/3 request: %v", err)
	}
	for name, value := range req.Headers {
		httpReq.Header.Set(name, value)
	}

	started := time.Now()
	res, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("http3 request failed: %v", err)
	}
	defer res.Body.Close()
	headers := map[string]string{}
	for name, values := range res.Header {
		headers[strings.ToLower(name)] = strings.Join(values, ", ")
	}
	limit := cfg.Limit()
	data, err := io.ReadAll(io.LimitReader(res.Body, int64(limit)+1))
	if err != nil {
		return nil, fmt.Errorf("http3 response read failed: %v", err)
	}
	if len(data) > limit {
		return nil, fmt.Errorf("response exceeds max_bytes (%d)", limit)
	}
	return &Response{
		Status:  res.StatusCode,
		MS:      time.Since(started).Milliseconds(),
		Headers: headers,
		Body:    FromBytes(data),
	}, nil
}

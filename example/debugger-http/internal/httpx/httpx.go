// Package httpx holds the HTTP plumbing shared by the HTTP-family
// example adapters (SOAP, GraphQL, OData, webhooks, HTTP/2, HTTP/3).
package httpx

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/erikwang2013/apidoc-go"
)

// MaxResponse caps the response body read by an adapter. The server
// enforces its own 4 MiB cap; this keeps a misbehaving target from
// filling memory before that cap applies.
const MaxResponse = 4 << 20

// NewRequest parses req.URL, rewrites protocol schemes (for example
// "soap" -> "http") and applies query/header params and headers.
func NewRequest(ctx context.Context, req apidoc.DebugRequest, method string, schemes map[string]string) (*http.Request, error) {
	u, err := url.Parse(req.URL)
	if err != nil || u.Host == "" {
		return nil, fmt.Errorf("bad url %q", req.URL)
	}
	scheme := strings.ToLower(u.Scheme)
	if to, ok := schemes[scheme]; ok {
		u.Scheme = to
	} else if scheme != "http" && scheme != "https" {
		return nil, fmt.Errorf("bad url scheme %q", u.Scheme)
	}

	q := u.Query()
	for _, p := range req.Params {
		if p.In == "query" {
			q.Set(p.Name, p.Value)
		}
	}
	u.RawQuery = q.Encode()

	hr, err := http.NewRequestWithContext(ctx, method, u.String(), bytes.NewReader(req.Body))
	if err != nil {
		return nil, err
	}
	for k, v := range req.Headers {
		hr.Header.Set(k, v)
	}
	for _, p := range req.Params {
		if p.In == "header" {
			hr.Header.Set(p.Name, p.Value)
		}
	}
	return hr, nil
}

// Do executes hr and converts the response. An empty encoding defaults
// to "text".
func Do(client *http.Client, hr *http.Request, encoding string) (*apidoc.DebugResponse, error) {
	start := time.Now()
	resp, err := client.Do(hr)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, MaxResponse+1))
	if err != nil {
		return nil, err
	}
	if len(body) > MaxResponse {
		body = body[:MaxResponse]
	}

	headers := make(map[string]string, len(resp.Header))
	for k, v := range resp.Header {
		headers[k] = strings.Join(v, ", ")
	}
	if encoding == "" {
		encoding = "text"
	}
	return &apidoc.DebugResponse{
		Status:   resp.Status,
		Headers:  headers,
		Body:     body,
		Encoding: encoding,
		Duration: time.Since(start),
	}, nil
}

// Timeout returns options["timeout_ms"], defaulting to 30s and capped
// at one minute.
func Timeout(opts map[string]string) time.Duration {
	d := 30 * time.Second
	if v := opts["timeout_ms"]; v != "" {
		if ms, err := strconv.Atoi(v); err == nil && ms > 0 {
			d = time.Duration(ms) * time.Millisecond
		}
	}
	if d > time.Minute {
		d = time.Minute
	}
	return d
}

// TLS builds the client TLS config from options. insecure_skip_verify
// is for local self-signed targets; ca_file adds a PEM root for
// private CAs.
func TLS(opts map[string]string) (*tls.Config, error) {
	cfg := &tls.Config{MinVersion: tls.VersionTLS12}
	if v := opts["insecure_skip_verify"]; v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return nil, fmt.Errorf("insecure_skip_verify: %w", err)
		}
		cfg.InsecureSkipVerify = b
	}
	if path := opts["ca_file"]; path != "" {
		pem, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("ca_file: %w", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pem) {
			return nil, fmt.Errorf("ca_file %q: no certificates found", path)
		}
		cfg.RootCAs = pool
	}
	return cfg, nil
}

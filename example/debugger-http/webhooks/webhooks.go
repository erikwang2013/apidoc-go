// Package webhooks implements a signed sample webhook POST adapter.
package webhooks

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/erikwang2013/apidoc-go"
	"github.com/erikwang2013/apidoc-go/example/debugger-http/internal/httpx"
)

var schemes = map[string]string{"webhook": "http", "webhooks": "https"}

const defaultBody = `{"event":"ping"}`

// New returns a debugger that POSTs a payload signed with
// HMAC-SHA256. Options: secret (required) is the HMAC key;
// signature_header overrides X-Hub-Signature-256. The header value is
// "sha256=<hex>".
func New() apidoc.Debugger {
	return apidoc.DebuggerFunc("webhooks", apidoc.DebugSpec{
		Mode:         "server",
		Actions:      []string{"POST"},
		Encodings:    []string{"json", "text"},
		BodyIn:       "payload",
		ResponseView: "http",
		ConnectFields: []apidoc.DebugField{
			{Name: "secret", Label: "HMAC secret", Type: "password", Required: true, Secret: true},
			{Name: "signature_header", Label: "Signature header", Default: "X-Hub-Signature-256"},
		},
	}, exec)
}

func exec(ctx context.Context, req apidoc.DebugRequest) (*apidoc.DebugResponse, error) {
	if req.Method != "" && strings.ToUpper(req.Method) != "POST" {
		return nil, fmt.Errorf("webhooks: unsupported method %q", req.Method)
	}
	secret := req.Options["secret"]
	if secret == "" {
		return nil, errors.New("webhooks: secret is required")
	}
	if len(req.Body) == 0 {
		req.Body = []byte(defaultBody)
	}

	hr, err := httpx.NewRequest(ctx, req, "POST", schemes)
	if err != nil {
		return nil, err
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(req.Body)
	header := req.Options["signature_header"]
	if header == "" {
		header = "X-Hub-Signature-256"
	}
	hr.Header.Set(header, "sha256="+hex.EncodeToString(mac.Sum(nil)))
	if hr.Header.Get("Content-Type") == "" {
		hr.Header.Set("Content-Type", "application/json")
	}

	client := &http.Client{Timeout: httpx.Timeout(req.Options)}
	return httpx.Do(client, hr, "text")
}

package webhooks

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/erikwang2013/apidoc-go"
)

func sign(secret, body string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(body))
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func TestWebhookSignature(t *testing.T) {
	const secret = "s3cret"
	const payload = `{"event":"order.created","id":42}`

	var gotSig, gotCT, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s", r.Method)
		}
		gotSig = r.Header.Get("X-Hub-Signature-256")
		gotCT = r.Header.Get("Content-Type")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	resp, err := New().Exec(context.Background(), apidoc.DebugRequest{
		URL:     srv.URL,
		Body:    []byte(payload),
		Options: map[string]string{"secret": secret},
	})
	if err != nil {
		t.Fatal(err)
	}
	if gotBody != payload {
		t.Errorf("body = %q", gotBody)
	}
	if want := sign(secret, payload); gotSig != want {
		t.Errorf("signature = %q, want %q", gotSig, want)
	}
	if !strings.HasPrefix(gotCT, "application/json") {
		t.Errorf("Content-Type = %q", gotCT)
	}
	if resp.Status != "202 Accepted" {
		t.Errorf("status = %q", resp.Status)
	}
}

func TestWebhookCustomHeaderAndDefaultBody(t *testing.T) {
	var gotSig string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotSig = r.Header.Get("X-Signature")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	_, err := New().Exec(context.Background(), apidoc.DebugRequest{
		URL: srv.URL,
		Options: map[string]string{
			"secret":           "k",
			"signature_header": "X-Signature",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if want := sign("k", defaultBody); gotSig != want {
		t.Errorf("signature = %q, want %q", gotSig, want)
	}
}

func TestWebhookErrors(t *testing.T) {
	if _, err := New().Exec(context.Background(), apidoc.DebugRequest{
		URL: "http://example.invalid",
	}); err == nil {
		t.Error("want error when secret is missing")
	}
	if _, err := New().Exec(context.Background(), apidoc.DebugRequest{
		URL: "http://example.invalid", Method: "GET",
		Options: map[string]string{"secret": "k"},
	}); err == nil {
		t.Error("want error for non-POST method")
	}
}

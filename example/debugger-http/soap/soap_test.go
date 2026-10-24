package soap

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/erikwang2013/apidoc-go"
)

const envelope = `<?xml version="1.0"?><soap:Envelope xmlns:soap="http://schemas.xmlsoap.org/soap/envelope/"><soap:Body><Add><a>1</a><b>2</b></Add></soap:Body></soap:Envelope>`

func TestSOAP11(t *testing.T) {
	var gotAction, gotCT, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s", r.Method)
		}
		gotAction = r.Header.Get("SOAPAction")
		gotCT = r.Header.Get("Content-Type")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.Header().Set("Content-Type", "text/xml")
		_, _ = io.WriteString(w, `<Envelope><Body><AddResponse>3</AddResponse></Body></Envelope>`)
	}))
	defer srv.Close()

	// soap:// must be rewritten to http:// by the adapter.
	resp, err := New().Exec(context.Background(), apidoc.DebugRequest{
		URL:      strings.Replace(srv.URL, "http://", "soap://", 1),
		Method:   "POST",
		Body:     []byte(envelope),
		Encoding: "xml",
		Options:  map[string]string{"soap_action": "urn:add"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if gotAction != `"urn:add"` {
		t.Errorf("SOAPAction = %q", gotAction)
	}
	if !strings.HasPrefix(gotCT, "text/xml") {
		t.Errorf("Content-Type = %q", gotCT)
	}
	if gotBody != envelope {
		t.Errorf("body = %q", gotBody)
	}
	if resp.Status != "200 OK" || !strings.Contains(string(resp.Body), "AddResponse") {
		t.Fatalf("response = %+v", resp)
	}
	if resp.Encoding != "xml" {
		t.Errorf("encoding = %q", resp.Encoding)
	}
}

func TestSOAP12(t *testing.T) {
	var gotAction, gotCT string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAction = r.Header.Get("SOAPAction")
		gotCT = r.Header.Get("Content-Type")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	_, err := New().Exec(context.Background(), apidoc.DebugRequest{
		URL:     srv.URL,
		Body:    []byte(envelope),
		Options: map[string]string{"soap_version": "1.2", "soap_action": "urn:add"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if gotAction != "" {
		t.Errorf("SOAP 1.2 must not send SOAPAction, got %q", gotAction)
	}
	if !strings.HasPrefix(gotCT, "application/soap+xml") || !strings.Contains(gotCT, `action="urn:add"`) {
		t.Errorf("Content-Type = %q", gotCT)
	}
}

func TestSOAPErrors(t *testing.T) {
	if _, err := New().Exec(context.Background(), apidoc.DebugRequest{
		URL: "http://example.invalid", Body: []byte(envelope),
		Options: map[string]string{"soap_version": "2.0"},
	}); err == nil {
		t.Error("want error for unknown SOAP version")
	}
	if _, err := New().Exec(context.Background(), apidoc.DebugRequest{
		URL: "http://example.invalid",
	}); err == nil {
		t.Error("want error for empty envelope")
	}
	if _, err := New().Exec(context.Background(), apidoc.DebugRequest{
		URL: "ftp://example.invalid", Body: []byte(envelope),
	}); err == nil {
		t.Error("want error for unsupported scheme")
	}
	if _, err := New().Exec(context.Background(), apidoc.DebugRequest{
		URL: "http://example.invalid", Method: "PUT", Body: []byte(envelope),
	}); err == nil {
		t.Error("want error for non-POST method")
	}
}

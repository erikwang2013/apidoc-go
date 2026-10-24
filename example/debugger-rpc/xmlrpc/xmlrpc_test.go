package xmlrpc

import (
	"context"
	"encoding/xml"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/erikwang2013/apidoc-go"
)

func TestCall(t *testing.T) {
	var got struct {
		MethodName string   `xml:"methodName"`
		Params     []xParam `xml:"params>param"`
	}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.Header.Get("Content-Type") != "text/xml" {
			t.Errorf("request = %s %s", r.Method, r.Header.Get("Content-Type"))
		}
		body, _ := io.ReadAll(r.Body)
		if err := xml.Unmarshal(body, &got); err != nil {
			t.Errorf("request XML: %v", err)
		}
		_, _ = w.Write([]byte(`<?xml version="1.0"?>
<methodResponse><params><param><value><struct>
<member><name>echo</name><value><string>hello</string></value></member>
<member><name>sum</name><value><int>7</int></value></member>
</struct></value></param></params></methodResponse>`))
	}))
	defer ts.Close()

	resp, err := New().Exec(context.Background(), apidoc.DebugRequest{
		URL:     "xmlrpc://" + strings.TrimPrefix(ts.URL, "http://") + "/rpc",
		Body:    []byte(`["hello",7]`),
		Options: map[string]string{"rpc_method": "demo.echo"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.MethodName != "demo.echo" || len(got.Params) != 2 {
		t.Fatalf("request = %+v", got)
	}
	if got.Params[0].Value.String == nil || *got.Params[0].Value.String != "hello" {
		t.Fatalf("first param = %+v", got.Params[0].Value)
	}
	if got.Params[1].Value.Int == nil || *got.Params[1].Value.Int != "7" {
		t.Fatalf("second param = %+v", got.Params[1].Value)
	}
	if resp.Status != "OK" || !strings.Contains(string(resp.Body), `"echo": "hello"`) || !strings.Contains(string(resp.Body), `"sum": 7`) {
		t.Fatalf("status=%q body=%s", resp.Status, resp.Body)
	}
}

func TestFault(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`<methodResponse><fault><value><struct>
<member><name>faultCode</name><value><int>4</int></value></member>
<member><name>faultString</name><value><string>boom</string></value></member>
</struct></value></fault></methodResponse>`))
	}))
	defer ts.Close()

	resp, err := New().Exec(context.Background(), apidoc.DebugRequest{
		URL:     "xmlrpc://" + strings.TrimPrefix(ts.URL, "http://") + "/rpc",
		Options: map[string]string{"rpc_method": "demo.echo"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Status != "ERROR" || !strings.Contains(string(resp.Body), "boom") {
		t.Fatalf("status=%q body=%s", resp.Status, resp.Body)
	}
}

func TestBuildCallRawFragment(t *testing.T) {
	call, err := buildCall("demo.echo", []byte(`<param><value><string>x</string></value></param>`))
	if err != nil {
		t.Fatal(err)
	}
	want := `<methodCall><methodName>demo.echo</methodName><params><param><value><string>x</string></value></param></params></methodCall>`
	if !strings.Contains(string(call), want) {
		t.Fatalf("call = %s", call)
	}
}

func TestBuildCallStructArrayAndDouble(t *testing.T) {
	call, err := buildCall("demo.echo", []byte(`[{"b":2,"a":[true,"x"]},2.5]`))
	if err != nil {
		t.Fatal(err)
	}
	s := string(call)
	if strings.Index(s, "<name>a</name>") > strings.Index(s, "<name>b</name>") {
		t.Fatalf("struct members are not sorted: %s", s)
	}
	for _, want := range []string{"<array><data>", "<boolean>1</boolean>", "<string>x</string>", "<double>2.5</double>"} {
		if !strings.Contains(s, want) {
			t.Fatalf("call is missing %q: %s", want, s)
		}
	}
}

func TestBuildCallRejectsNull(t *testing.T) {
	if _, err := buildCall("m", []byte(`[null]`)); err == nil {
		t.Fatal("want error for null parameter")
	}
}

func TestMethodRequired(t *testing.T) {
	if _, err := New().Exec(context.Background(), apidoc.DebugRequest{URL: "xmlrpc://localhost/rpc"}); err == nil {
		t.Fatal("want error when method is missing")
	}
}

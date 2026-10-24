// Package xmlrpc is an example XML-RPC over HTTP debug adapter.
//
// The body is either a JSON array of parameters (converted to XML-RPC values)
// or a raw <params> / <param> XML fragment. The RPC method comes from the
// rpc_method option, the ?method= query, or the action method when it is not
// CALL.
package xmlrpc

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/erikwang2013/apidoc-go"
)

const maxBody = 4 << 20

// New returns the XML-RPC adapter.
func New() apidoc.Debugger {
	return apidoc.DebuggerFunc("xmlrpc", apidoc.DebugSpec{
		Mode:         "server",
		Actions:      []string{"CALL"},
		Encodings:    []string{"json", "xml"},
		BodyIn:       "payload",
		ResponseView: "json",
		ConnectFields: []apidoc.DebugField{
			{Name: "rpc_method", Label: "RPC method", Type: "text", Required: true, Placeholder: "demo.echo"},
			{Name: "timeout_ms", Label: "Timeout (ms)", Type: "number", Default: "10000"},
		},
	}, exec)
}

func endpoint(raw string) (string, *url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", nil, fmt.Errorf("xmlrpc: bad url %q: %w", raw, err)
	}
	switch u.Scheme {
	case "xmlrpc":
		u.Scheme = "http"
	case "xmlrpcs":
		u.Scheme = "https"
	case "http", "https":
	default:
		return "", nil, fmt.Errorf("xmlrpc: url scheme must be xmlrpc, xmlrpcs, http or https, got %q", u.Scheme)
	}
	if u.Host == "" {
		return "", nil, fmt.Errorf("xmlrpc: url %q has no host", raw)
	}
	return u.String(), u, nil
}

func rpcMethod(req apidoc.DebugRequest, u *url.URL) string {
	if m := req.Options["rpc_method"]; m != "" {
		return m
	}
	if m := u.Query().Get("method"); m != "" {
		return m
	}
	if req.Method != "" && !strings.EqualFold(req.Method, "CALL") {
		return req.Method
	}
	return ""
}

func exec(ctx context.Context, req apidoc.DebugRequest) (*apidoc.DebugResponse, error) {
	start := time.Now()
	target, u, err := endpoint(req.URL)
	if err != nil {
		return nil, err
	}
	method := rpcMethod(req, u)
	if method == "" {
		return nil, fmt.Errorf("xmlrpc: set the rpc_method option or ?method= in the url")
	}
	call, err := buildCall(method, req.Body)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(call))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "text/xml")
	httpReq.Header.Set("Accept", "text/xml")
	for k, v := range req.Headers {
		httpReq.Header.Set(k, v)
	}
	timeout := 10 * time.Second
	if ms, err := strconv.Atoi(req.Options["timeout_ms"]); err == nil && ms > 0 {
		timeout = time.Duration(ms) * time.Millisecond
	}
	resp, err := (&http.Client{Timeout: timeout}).Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("xmlrpc: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return nil, fmt.Errorf("xmlrpc: read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("xmlrpc: HTTP %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}

	var out methodResponse
	if err := xml.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("xmlrpc: response is not XML-RPC: %w", err)
	}
	duration := time.Since(start)
	if out.Fault != nil {
		fault, _ := json.MarshalIndent(out.Fault.Value.any(), "", "  ")
		return &apidoc.DebugResponse{
			Status:   "ERROR",
			Headers:  map[string]string{"remote": u.Host, "method": method},
			Body:     fault,
			Encoding: "json",
			Duration: duration,
		}, nil
	}
	var result any
	if out.Params != nil && len(out.Params.Param) > 0 {
		result = out.Params.Param[0].Value.any()
	}
	pretty, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("xmlrpc: encode response: %w", err)
	}
	return &apidoc.DebugResponse{
		Status:   "OK",
		Headers:  map[string]string{"remote": u.Host, "method": method},
		Body:     pretty,
		Encoding: "json",
		Duration: duration,
	}, nil
}

func buildCall(method string, body []byte) ([]byte, error) {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0"?><methodCall><methodName>`)
	if err := xml.EscapeText(&b, []byte(method)); err != nil {
		return nil, err
	}
	b.WriteString(`</methodName>`)
	trim := bytes.TrimSpace(body)
	switch {
	case len(trim) == 0:
	case trim[0] == '<':
		if bytes.HasPrefix(trim, []byte("<params")) {
			b.Write(trim)
		} else {
			b.WriteString("<params>")
			b.Write(trim)
			b.WriteString("</params>")
		}
	default:
		var vals []any
		if err := json.Unmarshal(trim, &vals); err != nil {
			return nil, fmt.Errorf("xmlrpc: body must be a JSON array or an XML fragment: %w", err)
		}
		b.WriteString("<params>")
		for i, v := range vals {
			b.WriteString("<param>")
			if err := writeValue(&b, v); err != nil {
				return nil, fmt.Errorf("xmlrpc: parameter %d: %w", i, err)
			}
			b.WriteString("</param>")
		}
		b.WriteString("</params>")
	}
	b.WriteString("</methodCall>")
	return []byte(b.String()), nil
}

func writeValue(b *strings.Builder, v any) error {
	b.WriteString("<value>")
	switch x := v.(type) {
	case string:
		writeText(b, "string", x)
	case bool:
		s := "0"
		if x {
			s = "1"
		}
		writeText(b, "boolean", s)
	case float64:
		if x == math.Trunc(x) && math.Abs(x) < math.MaxInt32 {
			writeText(b, "int", strconv.FormatInt(int64(x), 10))
		} else {
			writeText(b, "double", strconv.FormatFloat(x, 'g', -1, 64))
		}
	case []any:
		b.WriteString("<array><data>")
		for _, e := range x {
			if err := writeValue(b, e); err != nil {
				return err
			}
		}
		b.WriteString("</data></array>")
	case map[string]any:
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		b.WriteString("<struct>")
		for _, k := range keys {
			b.WriteString("<member><name>")
			if err := xml.EscapeText(b, []byte(k)); err != nil {
				return err
			}
			b.WriteString("</name>")
			if err := writeValue(b, x[k]); err != nil {
				return err
			}
			b.WriteString("</member>")
		}
		b.WriteString("</struct>")
	case nil:
		return errors.New("XML-RPC has no null value")
	default:
		return fmt.Errorf("unsupported value type %T", v)
	}
	b.WriteString("</value>")
	return nil
}

func writeText(b *strings.Builder, tag, s string) {
	b.WriteString("<" + tag + ">")
	_ = xml.EscapeText(b, []byte(s))
	b.WriteString("</" + tag + ">")
}

// xValue decodes any XML-RPC <value> body. Exactly one field is set.
type xValue struct {
	String   *string   `xml:"string"`
	Int      *string   `xml:"int"`
	I4       *string   `xml:"i4"`
	I8       *string   `xml:"i8"`
	Boolean  *string   `xml:"boolean"`
	Double   *string   `xml:"double"`
	DateTime *string   `xml:"dateTime.iso8601"`
	Base64   *string   `xml:"base64"`
	Nil      *struct{} `xml:"nil"`
	Array    *xArray   `xml:"array"`
	Struct   *xStruct  `xml:"struct"`
}

type xArray struct {
	Values []xValue `xml:"data>value"`
}

type xStruct struct {
	Members []xMember `xml:"member"`
}

type xMember struct {
	Name  string `xml:"name"`
	Value xValue `xml:"value"`
}

func (v xValue) any() any {
	number := func(s string) any {
		if n, err := strconv.ParseInt(s, 10, 64); err == nil {
			return n
		}
		if f, err := strconv.ParseFloat(s, 64); err == nil {
			return f
		}
		return s
	}
	switch {
	case v.String != nil:
		return *v.String
	case v.Int != nil:
		return number(*v.Int)
	case v.I4 != nil:
		return number(*v.I4)
	case v.I8 != nil:
		return number(*v.I8)
	case v.Boolean != nil:
		return *v.Boolean == "1" || strings.EqualFold(*v.Boolean, "true")
	case v.Double != nil:
		return number(*v.Double)
	case v.DateTime != nil:
		return *v.DateTime
	case v.Base64 != nil:
		return *v.Base64
	case v.Nil != nil:
		return nil
	case v.Array != nil:
		out := make([]any, len(v.Array.Values))
		for i, e := range v.Array.Values {
			out[i] = e.any()
		}
		return out
	case v.Struct != nil:
		out := make(map[string]any, len(v.Struct.Members))
		for _, m := range v.Struct.Members {
			out[m.Name] = m.Value.any()
		}
		return out
	}
	return nil
}

type xParam struct {
	Value xValue `xml:"value"`
}

type methodResponse struct {
	Params *struct {
		Param []xParam `xml:"param"`
	} `xml:"params"`
	Fault *xParam `xml:"fault"`
}

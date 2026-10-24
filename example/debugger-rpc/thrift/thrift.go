// Package thriftdbg is an example Thrift debug adapter for the tiny service in
// echo.thrift. It speaks the Thrift binary/compact wire protocol through the
// official Go library, so no generated code is needed to send a call.
//
// The body is the string argument; the response is the returned string.
package thriftdbg

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/url"
	"strconv"
	"time"

	"github.com/apache/thrift/lib/go/thrift"
	"github.com/erikwang2013/apidoc-go"
)

// Method is the Thrift method declared in echo.thrift.
const Method = "echo"

// New returns the Thrift adapter.
func New() apidoc.Debugger {
	return apidoc.DebuggerFunc("thrift", apidoc.DebugSpec{
		Mode:         "server",
		Actions:      []string{"CALL"},
		Encodings:    []string{"text", "base64"},
		BodyIn:       "payload",
		ResponseView: "text",
		ConnectFields: []apidoc.DebugField{
			{Name: "timeout_ms", Label: "Timeout (ms)", Type: "number", Default: "5000"},
			{Name: "protocol", Label: "Wire protocol", Type: "select", Default: "binary", Options: []string{"binary", "compact"}},
		},
	}, exec)
}

func exec(ctx context.Context, req apidoc.DebugRequest) (*apidoc.DebugResponse, error) {
	start := time.Now()
	u, err := url.Parse(req.URL)
	if err != nil || u.Scheme != "thrift" || u.Host == "" {
		return nil, fmt.Errorf("thrift: bad url %q (want thrift://host:port)", req.URL)
	}
	timeout := 5 * time.Second
	if ms, err := strconv.Atoi(req.Options["timeout_ms"]); err == nil && ms > 0 {
		timeout = time.Duration(ms) * time.Millisecond
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	sock := thrift.NewTSocketConf(u.Host, &thrift.TConfiguration{
		ConnectTimeout: timeout,
		SocketTimeout:  timeout,
	})
	transport := thrift.NewTBufferedTransport(sock, 8192)
	if err := transport.Open(); err != nil {
		return nil, fmt.Errorf("thrift: connect %s: %w", u.Host, err)
	}
	defer transport.Close()

	var factory thrift.TProtocolFactory = thrift.NewTBinaryProtocolFactoryConf(nil)
	if req.Options["protocol"] == "compact" {
		factory = thrift.NewTCompactProtocolFactoryConf(nil)
	}
	client := thrift.NewTStandardClient(factory.GetProtocol(transport), factory.GetProtocol(transport))

	body := req.Body
	if req.Encoding == "base64" {
		body, err = base64.StdEncoding.DecodeString(string(req.Body))
		if err != nil {
			return nil, fmt.Errorf("thrift: body is not valid base64: %w", err)
		}
	}
	method := req.Options["rpc_method"]
	if method == "" {
		method = Method
	}
	result := &echoResult{}
	if _, err := client.Call(ctx, method, &echoArgs{Msg: string(body)}, result); err != nil {
		return nil, fmt.Errorf("thrift: %w", err)
	}
	return &apidoc.DebugResponse{
		Status:   "OK",
		Headers:  map[string]string{"remote": u.Host, "protocol": factoryName(req.Options)},
		Body:     []byte(result.Value),
		Encoding: "text",
		Duration: time.Since(start),
	}, nil
}

func factoryName(opts map[string]string) string {
	if opts["protocol"] == "compact" {
		return "compact"
	}
	return "binary"
}

// echoArgs is the generated-code-free equivalent of echo_args (field 1).
type echoArgs struct{ Msg string }

func (a *echoArgs) Write(ctx context.Context, p thrift.TProtocol) error {
	if err := p.WriteStructBegin(ctx, "echo_args"); err != nil {
		return err
	}
	if err := p.WriteFieldBegin(ctx, "msg", thrift.STRING, 1); err != nil {
		return err
	}
	if err := p.WriteString(ctx, a.Msg); err != nil {
		return err
	}
	if err := p.WriteFieldEnd(ctx); err != nil {
		return err
	}
	if err := p.WriteFieldStop(ctx); err != nil {
		return err
	}
	return p.WriteStructEnd(ctx)
}

func (a *echoArgs) Read(ctx context.Context, p thrift.TProtocol) error {
	if _, err := p.ReadStructBegin(ctx); err != nil {
		return err
	}
	for {
		_, typ, id, err := p.ReadFieldBegin(ctx)
		if err != nil {
			return err
		}
		if typ == thrift.STOP {
			break
		}
		if id == 1 && typ == thrift.STRING {
			if a.Msg, err = p.ReadString(ctx); err != nil {
				return err
			}
		} else if err := p.Skip(ctx, typ); err != nil {
			return err
		}
		if err := p.ReadFieldEnd(ctx); err != nil {
			return err
		}
	}
	return p.ReadStructEnd(ctx)
}

// echoResult is the generated-code-free equivalent of echo_result (field 0).
type echoResult struct{ Value string }

func (r *echoResult) Write(ctx context.Context, p thrift.TProtocol) error {
	if err := p.WriteStructBegin(ctx, "echo_result"); err != nil {
		return err
	}
	if err := p.WriteFieldBegin(ctx, "success", thrift.STRING, 0); err != nil {
		return err
	}
	if err := p.WriteString(ctx, r.Value); err != nil {
		return err
	}
	if err := p.WriteFieldEnd(ctx); err != nil {
		return err
	}
	if err := p.WriteFieldStop(ctx); err != nil {
		return err
	}
	return p.WriteStructEnd(ctx)
}

func (r *echoResult) Read(ctx context.Context, p thrift.TProtocol) error {
	if _, err := p.ReadStructBegin(ctx); err != nil {
		return err
	}
	for {
		_, typ, id, err := p.ReadFieldBegin(ctx)
		if err != nil {
			return err
		}
		if typ == thrift.STOP {
			break
		}
		if id == 0 && typ == thrift.STRING {
			if r.Value, err = p.ReadString(ctx); err != nil {
				return err
			}
		} else if err := p.Skip(ctx, typ); err != nil {
			return err
		}
		if err := p.ReadFieldEnd(ctx); err != nil {
			return err
		}
	}
	return p.ReadStructEnd(ctx)
}

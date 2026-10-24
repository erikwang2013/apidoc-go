package thriftdbg

import (
	"context"
	"testing"

	"github.com/apache/thrift/lib/go/thrift"
	"github.com/erikwang2013/apidoc-go"
)

// echoProcessor is the server side of echo.thrift. It reads echo_args and
// writes echo_result with the official protocol implementation.
type echoProcessor struct{}

func (echoProcessor) Process(ctx context.Context, in, out thrift.TProtocol) (bool, thrift.TException) {
	name, _, seq, err := in.ReadMessageBegin(ctx)
	if err != nil {
		return false, thrift.WrapTException(err)
	}
	args := &echoArgs{}
	if err := args.Read(ctx, in); err != nil {
		return false, thrift.WrapTException(err)
	}
	if err := in.ReadMessageEnd(ctx); err != nil {
		return false, thrift.WrapTException(err)
	}
	if err := out.WriteMessageBegin(ctx, name, thrift.REPLY, seq); err != nil {
		return false, thrift.WrapTException(err)
	}
	if err := (&echoResult{Value: args.Msg}).Write(ctx, out); err != nil {
		return false, thrift.WrapTException(err)
	}
	if err := out.WriteMessageEnd(ctx); err != nil {
		return false, thrift.WrapTException(err)
	}
	if err := out.Flush(ctx); err != nil {
		return false, thrift.WrapTException(err)
	}
	return true, nil
}

func (echoProcessor) ProcessorMap() map[string]thrift.TProcessorFunction { return nil }
func (echoProcessor) AddToProcessorMap(string, thrift.TProcessorFunction) {
}

func startServer(t *testing.T, compact bool) string {
	t.Helper()
	sock, err := thrift.NewTServerSocket("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	if err := sock.Listen(); err != nil {
		t.Fatal(err)
	}
	var protocolFactory thrift.TProtocolFactory = thrift.NewTBinaryProtocolFactoryConf(nil)
	if compact {
		protocolFactory = thrift.NewTCompactProtocolFactoryConf(nil)
	}
	server := thrift.NewTSimpleServer4(echoProcessor{}, sock, thrift.NewTBufferedTransportFactory(8192), protocolFactory)
	go func() { _ = server.Serve() }()
	t.Cleanup(func() { _ = server.Stop() })
	return sock.Addr().String()
}

func TestEcho(t *testing.T) {
	for _, tc := range []struct {
		name    string
		compact bool
		body    string
		enc     string
		want    string
	}{
		{"binary", false, "hello", "text", "hello"},
		{"compact", true, "hello", "text", "hello"},
		{"base64", false, "aGVsbG8=", "base64", "hello"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			opts := map[string]string{"timeout_ms": "5000"}
			if tc.compact {
				opts["protocol"] = "compact"
			}
			resp, err := New().Exec(context.Background(), apidoc.DebugRequest{
				URL:      "thrift://" + startServer(t, tc.compact),
				Body:     []byte(tc.body),
				Encoding: tc.enc,
				Options:  opts,
			})
			if err != nil {
				t.Fatal(err)
			}
			if resp.Status != "OK" || string(resp.Body) != tc.want {
				t.Fatalf("status=%q body=%q", resp.Status, resp.Body)
			}
		})
	}
}

func TestBadURL(t *testing.T) {
	if _, err := New().Exec(context.Background(), apidoc.DebugRequest{URL: "http://localhost"}); err == nil {
		t.Fatal("want error for non-thrift url")
	}
}

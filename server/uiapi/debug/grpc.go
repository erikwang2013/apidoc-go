//go:build apidoc_debug_grpc || apidoc_debug_all

package debug

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/reflection/grpc_reflection_v1"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"
)

func init() { RegisterExecutor("grpc", executeGRPC) }

// executeGRPC runs a unary call using a FileDescriptorSet supplied in
// opts.descriptor (base64) or fetched through grpc server reflection. The
// request body is protobuf JSON; fields and types come from the descriptor.
func executeGRPC(cfg Config, req *Request) (*Response, error) {
	started := time.Now()
	service := req.Opts["service"]
	methodName := req.Opts["method"]
	if service == "" {
		return nil, fmt.Errorf("grpc requires opts.service")
	}
	if methodName == "" {
		return nil, fmt.Errorf("grpc requires opts.method")
	}
	if req.Target == "" {
		return nil, fmt.Errorf("grpc requires a target")
	}
	body, err := req.Body.Decode()
	if err != nil {
		return nil, err
	}
	if len(body) > cfg.Limit() {
		return nil, fmt.Errorf("request body exceeds max_bytes (%d)", cfg.Limit())
	}

	conn, err := dialGRPC(cfg, req)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	ctx, cancel := context.WithTimeout(context.Background(), req.Timeout(cfg))
	defer cancel()
	ctx = grpcHeaders(ctx, req.Headers)

	files, err := grpcFiles(ctx, cfg, req, conn, service)
	if err != nil {
		return nil, err
	}
	desc, err := files.FindDescriptorByName(protoreflect.FullName(service))
	if err != nil {
		return nil, fmt.Errorf("grpc service `%s` not found in descriptor", service)
	}
	serviceDesc, ok := desc.(protoreflect.ServiceDescriptor)
	if !ok {
		return nil, fmt.Errorf("grpc `%s` is not a service", service)
	}
	method := serviceDesc.Methods().ByName(protoreflect.Name(methodName))
	if method == nil {
		return nil, fmt.Errorf("grpc method `%s` not found on `%s`", methodName, service)
	}
	if method.IsStreamingClient() || method.IsStreamingServer() {
		return nil, fmt.Errorf("grpc streaming methods are not supported in this build")
	}

	input := dynamicpb.NewMessage(method.Input())
	if len(body) > 0 {
		if err := protojson.Unmarshal(body, input); err != nil {
			return nil, fmt.Errorf("invalid grpc request JSON: %v", err)
		}
	}
	if proto.Size(input) > cfg.Limit() {
		return nil, fmt.Errorf("grpc request exceeds max_bytes (%d)", cfg.Limit())
	}
	output := dynamicpb.NewMessage(method.Output())
	path := "/" + service + "/" + methodName
	if err := conn.Invoke(ctx, path, input, output); err != nil {
		return nil, fmt.Errorf("grpc status %s: %s", status.Code(err), status.Convert(err).Message())
	}
	encoded, err := protojson.Marshal(output)
	if err != nil {
		return nil, fmt.Errorf("grpc response encode failed: %v", err)
	}
	if len(encoded) > cfg.Limit() {
		return nil, fmt.Errorf("grpc response exceeds max_bytes (%d)", cfg.Limit())
	}
	return &Response{
		Status:  0,
		MS:      time.Since(started).Milliseconds(),
		Headers: map[string]string{"x-grpc-status": "0"},
		Body:    Payload{Encoding: "utf8", Data: string(encoded)},
	}, nil
}

func grpcHeaders(ctx context.Context, headers map[string]string) context.Context {
	if len(headers) == 0 {
		return ctx
	}
	pairs := make([]string, 0, len(headers)*2)
	for name, value := range headers {
		pairs = append(pairs, strings.ToLower(name), value)
	}
	return metadata.NewOutgoingContext(ctx, metadata.Pairs(pairs...))
}

func grpcTarget(cfg Config, raw string) (*Target, bool, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, false, fmt.Errorf("invalid grpc target: %v", err)
	}
	var secure bool
	switch strings.ToLower(u.Scheme) {
	case "grpc", "http":
	case "grpcs", "https":
		secure = true
	default:
		return nil, false, fmt.Errorf("unsupported grpc target scheme `%s`; use grpc:// or grpcs://", u.Scheme)
	}
	host := u.Hostname()
	if host == "" {
		return nil, false, fmt.Errorf("grpc target requires a host")
	}
	port := u.Port()
	if port == "" {
		if secure {
			port = "443"
		} else {
			port = "80"
		}
	}
	addrs, err := CheckHost(cfg, host, port)
	if err != nil {
		return nil, false, err
	}
	return &Target{URL: u, Host: host, Port: port, Addrs: addrs}, secure, nil
}

func dialGRPC(cfg Config, req *Request) (*grpc.ClientConn, error) {
	target, secure, err := grpcTarget(cfg, req.Target)
	if err != nil {
		return nil, err
	}
	limit := cfg.Limit()
	opts := []grpc.DialOption{
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return target.DialContext(ctx, "tcp", "")
		}),
		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(limit+1), grpc.MaxCallSendMsgSize(limit+1)),
	}
	if secure {
		opts = append(opts, grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{
			ServerName: target.Host,
			MinVersion: tls.VersionTLS12,
		})))
	} else {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}
	address := net.JoinHostPort(target.Host, target.Port)
	return grpc.NewClient("passthrough:///"+address, opts...)
}

func grpcFiles(ctx context.Context, cfg Config, req *Request, conn *grpc.ClientConn, service string) (*protoregistry.Files, error) {
	if encoded := strings.TrimSpace(req.Opts["descriptor"]); encoded != "" {
		data, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			return nil, fmt.Errorf("invalid grpc descriptor base64: %v", err)
		}
		if len(data) > cfg.Limit() {
			return nil, fmt.Errorf("grpc descriptor exceeds max_bytes (%d)", cfg.Limit())
		}
		set := &descriptorpb.FileDescriptorSet{}
		if err := proto.Unmarshal(data, set); err != nil {
			return nil, fmt.Errorf("invalid grpc descriptor: %v", err)
		}
		files, err := protodesc.NewFiles(set)
		if err != nil {
			return nil, fmt.Errorf("invalid grpc descriptor: %v", err)
		}
		return files, nil
	}

	stream, err := grpc_reflection_v1.NewServerReflectionClient(conn).ServerReflectionInfo(ctx)
	if err != nil {
		return nil, fmt.Errorf("grpc reflection failed: %v", err)
	}
	defer stream.CloseSend()
	if err := stream.Send(&grpc_reflection_v1.ServerReflectionRequest{
		MessageRequest: &grpc_reflection_v1.ServerReflectionRequest_FileContainingSymbol{FileContainingSymbol: service},
	}); err != nil {
		return nil, fmt.Errorf("grpc reflection failed: %v", err)
	}
	response, err := stream.Recv()
	if err != nil {
		return nil, fmt.Errorf("grpc reflection failed: %v", err)
	}
	found := response.GetFileDescriptorResponse()
	if found == nil {
		return nil, fmt.Errorf("grpc reflection returned an unexpected response; set opts.descriptor")
	}
	set := &descriptorpb.FileDescriptorSet{}
	total := 0
	for _, raw := range found.FileDescriptorProto {
		total += len(raw)
		if total > cfg.Limit() {
			return nil, fmt.Errorf("grpc reflection descriptor exceeds max_bytes (%d)", cfg.Limit())
		}
		file := &descriptorpb.FileDescriptorProto{}
		if err := proto.Unmarshal(raw, file); err != nil {
			return nil, fmt.Errorf("grpc reflection descriptor decode failed: %v", err)
		}
		set.File = append(set.File, file)
	}
	files, err := protodesc.NewFiles(set)
	if err != nil {
		return nil, fmt.Errorf("grpc reflection descriptor invalid: %v", err)
	}
	return files, nil
}

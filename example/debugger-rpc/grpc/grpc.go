// Package grpcdbg is an example gRPC debug adapter.
//
// It resolves the method's input/output descriptors through gRPC server
// reflection and uses dynamicpb, so any service that enables reflection can
// be debugged without generated code. The JSON body is one request message.
//
// Unary methods work through Exec. StreamDebugger.Open covers all four stream
// shapes: Send appends request messages and Events yields the responses.
package grpcdbg

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/erikwang2013/apidoc-go"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	reflectionpb "google.golang.org/grpc/reflection/grpc_reflection_v1"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"
)

var errSessionClosed = errors.New("grpc: session closed")

// New returns the gRPC adapter. It is both a Debugger and a StreamDebugger.
func New() apidoc.StreamDebugger { return adapter{} }

type adapter struct{}

func (adapter) Protocol() string { return "grpc" }

func (adapter) Spec() apidoc.DebugSpec {
	return apidoc.DebugSpec{
		Mode:         "server",
		Actions:      []string{"CALL"},
		Encodings:    []string{"json"},
		BodyIn:       "payload",
		ResponseView: "transcript",
		Streaming:    true,
		ConnectFields: []apidoc.DebugField{
			{Name: "timeout_ms", Label: "Timeout (ms)", Type: "number", Default: "10000"},
		},
	}
}

type target struct {
	addr    string // host:port
	service string // fully-qualified service name
	method  string // method name
	tls     bool
}

func (t target) path() string { return "/" + t.service + "/" + t.method }

// parseTarget accepts grpc://host:port/pkg.Service/Method and the grpcs://
// TLS variant.
func parseTarget(raw string) (target, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return target{}, fmt.Errorf("grpc: bad url %q: %w", raw, err)
	}
	if u.Scheme != "grpc" && u.Scheme != "grpcs" {
		return target{}, fmt.Errorf("grpc: url scheme must be grpc or grpcs, got %q", u.Scheme)
	}
	if u.Host == "" {
		return target{}, fmt.Errorf("grpc: url %q has no host", raw)
	}
	p := strings.Trim(u.Path, "/")
	i := strings.LastIndexByte(p, '/')
	if i <= 0 || i == len(p)-1 {
		return target{}, fmt.Errorf("grpc: url path must be /<service>/<method>, got %q", u.Path)
	}
	addr := u.Host
	if _, _, err := net.SplitHostPort(addr); err != nil {
		addr = net.JoinHostPort(strings.Trim(addr, "[]"), "443")
	}
	return target{addr: addr, service: p[:i], method: p[i+1:], tls: u.Scheme == "grpcs"}, nil
}

func dialTarget(t target) (*grpc.ClientConn, error) {
	var creds credentials.TransportCredentials
	if t.tls {
		host, _, _ := net.SplitHostPort(t.addr)
		creds = credentials.NewTLS(&tls.Config{ServerName: host, MinVersion: tls.VersionTLS12})
	} else {
		creds = insecure.NewCredentials()
	}
	cc, err := grpc.NewClient(t.addr, grpc.WithTransportCredentials(creds))
	if err != nil {
		return nil, fmt.Errorf("grpc: dial %s: %w", t.addr, err)
	}
	return cc, nil
}

func timeoutFrom(opts map[string]string, def time.Duration) time.Duration {
	if ms, err := strconv.Atoi(opts["timeout_ms"]); err == nil && ms > 0 {
		return time.Duration(ms) * time.Millisecond
	}
	return def
}

// resolveMethod asks the server's reflection service for the file that
// declares the service, then walks to the method descriptor.
func resolveMethod(ctx context.Context, cc *grpc.ClientConn, t target) (protoreflect.MethodDescriptor, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	stream, err := reflectionpb.NewServerReflectionClient(cc).ServerReflectionInfo(ctx)
	if err != nil {
		return nil, fmt.Errorf("grpc: reflection: %w", err)
	}
	defer stream.CloseSend()
	if err := stream.Send(&reflectionpb.ServerReflectionRequest{
		MessageRequest: &reflectionpb.ServerReflectionRequest_FileContainingSymbol{FileContainingSymbol: t.service},
	}); err != nil {
		return nil, fmt.Errorf("grpc: reflection: %w", err)
	}
	resp, err := stream.Recv()
	if err != nil {
		return nil, fmt.Errorf("grpc: reflection: %w (does the server enable reflection?)", err)
	}
	if er := resp.GetErrorResponse(); er != nil {
		return nil, fmt.Errorf("grpc: reflection: %s (code %d)", er.GetErrorMessage(), er.GetErrorCode())
	}
	fdResp := resp.GetFileDescriptorResponse()
	if fdResp == nil {
		return nil, errors.New("grpc: reflection returned no file descriptors")
	}
	set := &descriptorpb.FileDescriptorSet{}
	for _, raw := range fdResp.GetFileDescriptorProto() {
		fd := &descriptorpb.FileDescriptorProto{}
		if err := proto.Unmarshal(raw, fd); err != nil {
			return nil, fmt.Errorf("grpc: reflection descriptor: %w", err)
		}
		set.File = append(set.File, fd)
	}
	files, err := protodesc.NewFiles(set)
	if err != nil {
		return nil, fmt.Errorf("grpc: reflection descriptors: %w", err)
	}
	d, err := files.FindDescriptorByName(protoreflect.FullName(t.service))
	if err != nil {
		return nil, fmt.Errorf("grpc: service %q: %w", t.service, err)
	}
	sd, ok := d.(protoreflect.ServiceDescriptor)
	if !ok {
		return nil, fmt.Errorf("grpc: %q is not a service", t.service)
	}
	md := sd.Methods().ByName(protoreflect.Name(t.method))
	if md == nil {
		return nil, fmt.Errorf("grpc: service %q has no method %q", t.service, t.method)
	}
	return md, nil
}

func jsonMessage(md protoreflect.MessageDescriptor, body []byte) (*dynamicpb.Message, error) {
	m := dynamicpb.NewMessage(md)
	if len(bytes.TrimSpace(body)) == 0 {
		return m, nil
	}
	if err := protojson.Unmarshal(body, m); err != nil {
		return nil, fmt.Errorf("grpc: body is not valid JSON for %s: %w", md.FullName(), err)
	}
	return m, nil
}

func marshalMessage(m *dynamicpb.Message) ([]byte, error) {
	return protojson.MarshalOptions{Multiline: true, Indent: "  ", EmitUnpopulated: true}.Marshal(m)
}

func (adapter) Exec(ctx context.Context, req apidoc.DebugRequest) (*apidoc.DebugResponse, error) {
	start := time.Now()
	t, err := parseTarget(req.URL)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, timeoutFrom(req.Options, 30*time.Second))
	defer cancel()
	cc, err := dialTarget(t)
	if err != nil {
		return nil, err
	}
	defer cc.Close()
	md, err := resolveMethod(ctx, cc, t)
	if err != nil {
		return nil, err
	}
	in, err := jsonMessage(md.Input(), req.Body)
	if err != nil {
		return nil, err
	}
	out := dynamicpb.NewMessage(md.Output())
	err = cc.Invoke(ctx, t.path(), in, out)
	duration := time.Since(start)
	st := status.Convert(err)
	if err != nil && st.Code() == codes.OK {
		return nil, fmt.Errorf("grpc: invoke: %w", err)
	}
	if st.Code() != codes.OK {
		return &apidoc.DebugResponse{
			Status:   st.Code().String(),
			Headers:  map[string]string{"grpc-status": strconv.Itoa(int(st.Code())), "target": t.addr},
			Body:     []byte(st.Message()),
			Encoding: "text",
			Duration: duration,
		}, nil
	}
	body, err := marshalMessage(out)
	if err != nil {
		return nil, err
	}
	return &apidoc.DebugResponse{
		Status:   "OK",
		Headers:  map[string]string{"grpc-status": "0", "target": t.addr},
		Body:     body,
		Encoding: "json",
		Duration: duration,
	}, nil
}

// Open resolves the method and prepares a session. The stream itself is
// created on the first Send so that per-message metadata can be attached.
func (adapter) Open(ctx context.Context, req apidoc.DebugRequest) (apidoc.Session, error) {
	t, err := parseTarget(req.URL)
	if err != nil {
		return nil, err
	}
	setupCtx, cancelSetup := context.WithTimeout(ctx, timeoutFrom(req.Options, 15*time.Second))
	defer cancelSetup()
	cc, err := dialTarget(t)
	if err != nil {
		return nil, err
	}
	md, err := resolveMethod(setupCtx, cc, t)
	if err != nil {
		cc.Close()
		return nil, err
	}
	sctx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	s := &session{
		cc:      cc,
		md:      md,
		method:  t.path(),
		headers: req.Headers,
		ctx:     sctx,
		cancel:  cancel,
		events:  make(chan apidoc.DebugEvent, 64),
		ready:   make(chan struct{}),
		done:    make(chan struct{}),
	}
	go s.run()
	return s, nil
}

type session struct {
	cc      *grpc.ClientConn
	md      protoreflect.MethodDescriptor
	method  string
	headers map[string]string
	ctx     context.Context
	cancel  context.CancelFunc

	events chan apidoc.DebugEvent
	ready  chan struct{}
	done   chan struct{}

	start    sync.Once
	startErr error
	stream   grpc.ClientStream

	sendMu     sync.Mutex
	sent       int
	sendClosed bool
	closeOne   sync.Once

	evMu     sync.RWMutex
	evClosed bool
}

func (s *session) ensureStream(meta map[string]string) error {
	s.start.Do(func() {
		out := metadata.New(nil)
		for k, v := range s.headers {
			out.Set(strings.ToLower(k), v)
		}
		for k, v := range meta {
			out.Set(strings.ToLower(k), v)
		}
		desc := &grpc.StreamDesc{
			ClientStreams: s.md.IsStreamingClient(),
			ServerStreams: s.md.IsStreamingServer(),
		}
		s.stream, s.startErr = s.cc.NewStream(metadata.NewOutgoingContext(s.ctx, out), desc, s.method)
		close(s.ready)
	})
	return s.startErr
}

// Send sends one request message. Unary and server-streaming methods accept
// exactly one; client-streaming and bidi methods accept any number.
func (s *session) Send(ctx context.Context, msg []byte, meta map[string]string) error {
	s.sendMu.Lock()
	defer s.sendMu.Unlock()
	if s.sendClosed {
		return errSessionClosed
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if s.sent > 0 && !s.md.IsStreamingClient() {
		return errors.New("grpc: this method accepts a single request message")
	}
	if err := s.ensureStream(meta); err != nil {
		return err
	}
	in, err := jsonMessage(s.md.Input(), msg)
	if err != nil {
		return err
	}
	if err := s.stream.SendMsg(in); err != nil {
		return err
	}
	s.sent++
	s.emit(apidoc.DebugEvent{Time: time.Now(), Direction: "out", Name: "message", Data: string(msg), Encoding: "json"})
	if !s.md.IsStreamingClient() {
		return s.stream.CloseSend()
	}
	return nil
}

func (s *session) Events() <-chan apidoc.DebugEvent { return s.events }

// Close half-closes client streams and waits briefly for the final response;
// server-streaming sessions are canceled immediately.
func (s *session) Close() error {
	s.closeOne.Do(func() {
		s.sendMu.Lock()
		defer s.sendMu.Unlock()
		s.sendClosed = true
		s.start.Do(func() {
			s.startErr = errSessionClosed
			close(s.ready)
		})
		if s.stream != nil && s.md.IsStreamingClient() {
			_ = s.stream.CloseSend()
			select {
			case <-s.done:
			case <-time.After(3 * time.Second):
			}
		}
		s.cancel()
		_ = s.cc.Close()
	})
	return nil
}

func (s *session) run() {
	defer close(s.done)
	defer s.closeEvents()
	<-s.ready
	if s.startErr != nil {
		if !errors.Is(s.startErr, errSessionClosed) {
			s.emit(apidoc.DebugEvent{Time: time.Now(), Direction: "in", Name: "status", Data: s.startErr.Error(), Encoding: "text"})
		}
		return
	}
	for {
		out := dynamicpb.NewMessage(s.md.Output())
		err := s.stream.RecvMsg(out)
		if err == io.EOF {
			s.emit(apidoc.DebugEvent{Time: time.Now(), Direction: "in", Name: "status", Data: "OK", Encoding: "text"})
			return
		}
		if err != nil {
			if c := status.Code(err); c != codes.Canceled {
				s.emit(apidoc.DebugEvent{Time: time.Now(), Direction: "in", Name: "status", Data: status.Convert(err).String(), Encoding: "text"})
			}
			return
		}
		body, err := marshalMessage(out)
		if err != nil {
			s.emit(apidoc.DebugEvent{Time: time.Now(), Direction: "in", Name: "status", Data: err.Error(), Encoding: "text"})
			return
		}
		if !s.emit(apidoc.DebugEvent{Time: time.Now(), Direction: "in", Name: "message", Data: string(body), Encoding: "json"}) {
			return
		}
	}
}

func (s *session) emit(ev apidoc.DebugEvent) bool {
	s.evMu.RLock()
	defer s.evMu.RUnlock()
	if s.evClosed {
		return false
	}
	select {
	case s.events <- ev:
		return true
	case <-s.ctx.Done():
		return false
	}
}

func (s *session) closeEvents() {
	s.evMu.Lock()
	s.evClosed = true
	close(s.events)
	s.evMu.Unlock()
}

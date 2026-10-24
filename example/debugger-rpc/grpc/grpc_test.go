package grpcdbg

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/erikwang2013/apidoc-go"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"
)

var (
	svcOnce sync.Once
	svcDesc protoreflect.ServiceDescriptor
	svcErr  error
)

func testService(t *testing.T) protoreflect.ServiceDescriptor {
	t.Helper()
	svcOnce.Do(buildTestService)
	if svcErr != nil {
		t.Fatal(svcErr)
	}
	return svcDesc
}

func buildTestService() {
	field := func(name string, num int32, typ descriptorpb.FieldDescriptorProto_Type) *descriptorpb.FieldDescriptorProto {
		return &descriptorpb.FieldDescriptorProto{
			Name:     proto.String(name),
			Number:   proto.Int32(num),
			Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
			Type:     typ.Enum(),
			JsonName: proto.String(name),
		}
	}
	fdp := &descriptorpb.FileDescriptorProto{
		Name:    proto.String("echo.proto"),
		Package: proto.String("echo"),
		Syntax:  proto.String("proto3"),
		MessageType: []*descriptorpb.DescriptorProto{
			{Name: proto.String("EchoRequest"), Field: []*descriptorpb.FieldDescriptorProto{
				field("text", 1, descriptorpb.FieldDescriptorProto_TYPE_STRING),
				field("count", 2, descriptorpb.FieldDescriptorProto_TYPE_INT32),
			}},
			{Name: proto.String("EchoResponse"), Field: []*descriptorpb.FieldDescriptorProto{
				field("text", 1, descriptorpb.FieldDescriptorProto_TYPE_STRING),
				field("index", 2, descriptorpb.FieldDescriptorProto_TYPE_INT32),
			}},
		},
		Service: []*descriptorpb.ServiceDescriptorProto{{
			Name: proto.String("EchoService"),
			Method: []*descriptorpb.MethodDescriptorProto{
				{Name: proto.String("Echo"), InputType: proto.String(".echo.EchoRequest"), OutputType: proto.String(".echo.EchoResponse")},
				{Name: proto.String("Expand"), InputType: proto.String(".echo.EchoRequest"), OutputType: proto.String(".echo.EchoResponse"), ServerStreaming: proto.Bool(true)},
				{Name: proto.String("Collect"), InputType: proto.String(".echo.EchoRequest"), OutputType: proto.String(".echo.EchoResponse"), ClientStreaming: proto.Bool(true)},
				{Name: proto.String("Chat"), InputType: proto.String(".echo.EchoRequest"), OutputType: proto.String(".echo.EchoResponse"), ClientStreaming: proto.Bool(true), ServerStreaming: proto.Bool(true)},
			},
		}},
	}
	f, err := protodesc.NewFile(fdp, nil)
	if err != nil {
		svcErr = err
		return
	}
	if err := protoregistry.GlobalFiles.RegisterFile(f); err != nil {
		svcErr = err
		return
	}
	d, err := protoregistry.GlobalFiles.FindDescriptorByName("echo.EchoService")
	if err != nil {
		svcErr = err
		return
	}
	sd, ok := d.(protoreflect.ServiceDescriptor)
	if !ok {
		svcErr = fmt.Errorf("echo.EchoService is not a service")
		return
	}
	svcDesc = sd
}

func response(md protoreflect.MessageDescriptor, text string, index int32) *dynamicpb.Message {
	m := dynamicpb.NewMessage(md)
	m.Set(md.Fields().ByName("text"), protoreflect.ValueOfString(text))
	m.Set(md.Fields().ByName("index"), protoreflect.ValueOfInt32(index))
	return m
}

// startEchoServer registers a hand-written dynamic service and reflection, so
// the adapter's descriptor lookup and streaming paths run against real gRPC.
func startEchoServer(t *testing.T) string {
	t.Helper()
	svc := testService(t)
	reqD := svc.Methods().ByName("Echo").Input()
	respD := svc.Methods().ByName("Echo").Output()
	text := func(m protoreflect.Message) string {
		return m.Get(reqD.Fields().ByName("text")).String()
	}
	count := func(m protoreflect.Message) int {
		return int(m.Get(reqD.Fields().ByName("count")).Int())
	}

	srv := grpc.NewServer()
	sd := grpc.ServiceDesc{
		ServiceName: string(svc.FullName()),
		HandlerType: (*any)(nil),
		Methods: []grpc.MethodDesc{{
			MethodName: "Echo",
			Handler: func(_ any, _ context.Context, dec func(any) error, _ grpc.UnaryServerInterceptor) (any, error) {
				in := dynamicpb.NewMessage(reqD)
				if err := dec(in); err != nil {
					return nil, err
				}
				return response(respD, text(in), 0), nil
			},
		}},
		Streams: []grpc.StreamDesc{
			{
				StreamName:    "Expand",
				ServerStreams: true,
				Handler: func(_ any, stream grpc.ServerStream) error {
					in := dynamicpb.NewMessage(reqD)
					if err := stream.RecvMsg(in); err != nil {
						return err
					}
					for i := 0; i < count(in) && i < 10; i++ {
						if err := stream.SendMsg(response(respD, text(in), int32(i))); err != nil {
							return err
						}
					}
					return nil
				},
			},
			{
				StreamName:    "Collect",
				ClientStreams: true,
				Handler: func(_ any, stream grpc.ServerStream) error {
					n := 0
					for {
						in := dynamicpb.NewMessage(reqD)
						if err := stream.RecvMsg(in); err != nil {
							break
						}
						n++
					}
					return stream.SendMsg(response(respD, "collected", int32(n)))
				},
			},
			{
				StreamName:    "Chat",
				ClientStreams: true,
				ServerStreams: true,
				Handler: func(_ any, stream grpc.ServerStream) error {
					i := 0
					for {
						in := dynamicpb.NewMessage(reqD)
						if err := stream.RecvMsg(in); err != nil {
							if err == io.EOF {
								return nil
							}
							return err
						}
						if err := stream.SendMsg(response(respD, text(in), int32(i))); err != nil {
							return err
						}
						i++
					}
				},
			},
		},
	}
	srv.RegisterService(&sd, nil)
	reflection.Register(srv)

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)
	return lis.Addr().String()
}

func TestExecUnary(t *testing.T) {
	addr := startEchoServer(t)
	resp, err := New().Exec(context.Background(), apidoc.DebugRequest{
		URL:  "grpc://" + addr + "/echo.EchoService/Echo",
		Body: []byte(`{"text":"hello","count":1}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Status != "OK" || !strings.Contains(string(resp.Body), `"hello"`) {
		t.Fatalf("status=%q body=%s", resp.Status, resp.Body)
	}
	if resp.Encoding != "json" {
		t.Fatalf("encoding = %q", resp.Encoding)
	}
}

func TestExecUnknownMethod(t *testing.T) {
	addr := startEchoServer(t)
	_, err := New().Exec(context.Background(), apidoc.DebugRequest{URL: "grpc://" + addr + "/echo.EchoService/Nope"})
	if err == nil {
		t.Fatal("want error for unknown method")
	}
}

func TestStreamServerStreaming(t *testing.T) {
	addr := startEchoServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	s, err := New().Open(ctx, apidoc.DebugRequest{
		URL:     "grpc://" + addr + "/echo.EchoService/Expand",
		Options: map[string]string{"timeout_ms": "10000"},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.Send(ctx, []byte(`{"text":"tick","count":3}`), nil); err != nil {
		t.Fatal(err)
	}
	var msgs []string
	status := ""
	for ev := range s.Events() {
		switch ev.Name {
		case "message":
			if ev.Direction == "in" {
				msgs = append(msgs, ev.Data)
			}
		case "status":
			status = ev.Data
		}
	}
	if len(msgs) != 3 {
		t.Fatalf("got %d messages: %v", len(msgs), msgs)
	}
	if status != "OK" {
		t.Fatalf("status event = %q", status)
	}
	var last struct {
		Index int32 `json:"index"`
	}
	if err := json.Unmarshal([]byte(msgs[2]), &last); err != nil || last.Index != 2 {
		t.Fatalf("third message = %s (%v)", msgs[2], err)
	}
}

func TestStreamBidi(t *testing.T) {
	addr := startEchoServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	s, err := New().Open(ctx, apidoc.DebugRequest{URL: "grpc://" + addr + "/echo.EchoService/Chat"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Send(ctx, []byte(`{"text":"one"}`), nil); err != nil {
		t.Fatal(err)
	}
	if err := s.Send(ctx, []byte(`{"text":"two"}`), nil); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	var got []string
	for ev := range s.Events() {
		if ev.Name == "message" && ev.Direction == "in" {
			got = append(got, ev.Data)
		}
	}
	if len(got) != 2 || !strings.Contains(got[0], "one") || !strings.Contains(got[1], "two") {
		t.Fatalf("messages = %v", got)
	}
}

func TestStreamClientStreaming(t *testing.T) {
	addr := startEchoServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	s, err := New().Open(ctx, apidoc.DebugRequest{URL: "grpc://" + addr + "/echo.EchoService/Collect"})
	if err != nil {
		t.Fatal(err)
	}
	for _, msg := range []string{`{"text":"a"}`, `{"text":"b"}`} {
		if err := s.Send(ctx, []byte(msg), nil); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	var last string
	for ev := range s.Events() {
		if ev.Name == "message" && ev.Direction == "in" {
			last = ev.Data
		}
	}
	var collected struct {
		Text  string `json:"text"`
		Index int32  `json:"index"`
	}
	if err := json.Unmarshal([]byte(last), &collected); err != nil || collected.Index != 2 || collected.Text != "collected" {
		t.Fatalf("response = %q (%v)", last, err)
	}
}

func TestParseTargetRequiresMethod(t *testing.T) {
	if _, err := parseTarget("grpc://localhost:50051/echo.EchoService"); err == nil {
		t.Fatal("want error for missing method")
	}
	if _, err := parseTarget("http://localhost:50051/echo.EchoService/Echo"); err == nil {
		t.Fatal("want error for wrong scheme")
	}
}

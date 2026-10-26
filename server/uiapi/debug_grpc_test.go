//go:build apidoc_debug_grpc || apidoc_debug_all

package uiapi

import (
	"context"
	"encoding/base64"
	"net"
	"strings"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
	testpb "google.golang.org/grpc/reflection/grpc_testing"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/types/descriptorpb"
)

type searchServer struct {
	testpb.UnimplementedSearchServiceServer
}

func (s *searchServer) Search(_ context.Context, req *testpb.SearchRequest) (*testpb.SearchResponse, error) {
	return &testpb.SearchResponse{Results: []*testpb.SearchResponse_Result{{
		Url:   "https://example.test/" + req.GetQuery(),
		Title: "found",
	}}}, nil
}

func startSearchServer(t *testing.T, withReflection bool) string {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := grpc.NewServer()
	testpb.RegisterSearchServiceServer(server, &searchServer{})
	if withReflection {
		reflection.Register(server)
	}
	go func() { _ = server.Serve(lis) }()
	t.Cleanup(server.Stop)
	return lis.Addr().String()
}

func TestRelayGRPCReflectionUnary(t *testing.T) {
	target := startSearchServer(t, true)
	h, id := debugRPCHandler(t, "grpc", "POST", "grpc://"+target, nil, nil,
		DebugConfig{EnableServerSide: true, AllowPrivateNetworks: true, TimeoutMS: 3000},
		map[string]string{"service": "grpc.testing.SearchService", "method": "Search"})
	data := debugSendData(t, h, id, `{"payload":"{\"query\":\"cats\"}","encoding":"utf8"}`)
	if data["ok"] != true {
		t.Fatalf("grpc reflection: %+v", data)
	}
	body := data["body"].(string)
	if !strings.Contains(body, `"title":"found"`) || !strings.Contains(body, "cats") {
		t.Fatalf("grpc body = %s", body)
	}
	if data["headers"].(map[string]any)["x-grpc-status"] != "0" {
		t.Fatalf("grpc headers = %+v", data["headers"])
	}
}

func TestRelayGRPCDescriptorUnary(t *testing.T) {
	target := startSearchServer(t, false)
	set := &descriptorpb.FileDescriptorSet{File: []*descriptorpb.FileDescriptorProto{
		protodesc.ToFileDescriptorProto(testpb.File_reflection_grpc_testing_test_proto),
	}}
	raw, err := proto.Marshal(set)
	if err != nil {
		t.Fatal(err)
	}
	h, id := debugRPCHandler(t, "grpc", "POST", "grpc://"+target, nil, nil,
		DebugConfig{EnableServerSide: true, AllowPrivateNetworks: true, TimeoutMS: 3000},
		map[string]string{
			"service": "grpc.testing.SearchService", "method": "Search",
			"descriptor": base64.StdEncoding.EncodeToString(raw),
		})
	data := debugSendData(t, h, id, `{"payload":"{\"query\":\"dogs\"}","encoding":"utf8"}`)
	if data["ok"] != true || !strings.Contains(data["body"].(string), "dogs") {
		t.Fatalf("grpc descriptor: %+v", data)
	}
}

func TestRelayGRPCFailures(t *testing.T) {
	target := startSearchServer(t, false)
	h, id := debugRPCHandler(t, "grpc", "POST", "grpc://"+target, nil, nil,
		DebugConfig{EnableServerSide: true, AllowPrivateNetworks: true, TimeoutMS: 1000},
		map[string]string{"service": "grpc.testing.SearchService", "method": "Search"})
	data := debugSendData(t, h, id, `{"payload":"{}","encoding":"utf8"}`)
	if data["ok"] != false || !strings.Contains(data["error"].(string), "reflection") {
		t.Fatalf("missing reflection: %+v", data)
	}

	blocked, blockedID := debugRPCHandler(t, "grpc", "POST", "grpc://127.0.0.1:9", nil, nil,
		DebugConfig{EnableServerSide: true, TimeoutMS: 500},
		map[string]string{"service": "grpc.testing.SearchService", "method": "Search"})
	data = debugSendData(t, blocked, blockedID, `{"payload":"{}","encoding":"utf8"}`)
	if data["ok"] != false || !strings.Contains(data["error"].(string), "private") {
		t.Fatalf("grpc ssrf: %+v", data)
	}
}

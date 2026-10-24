package opcua

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"testing"

	"github.com/erikwang2013/apidoc-go"
	"github.com/gopcua/opcua/id"
	"github.com/gopcua/opcua/server"
	"github.com/gopcua/opcua/ua"
)

func startOPCUA(t *testing.T) (endpoint, nodeID, objectsID string) {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()

	s := server.New(
		server.EndPoint("127.0.0.1", port),
		server.EnableSecurity("None", ua.MessageSecurityModeNone),
		server.EnableAuthMode(ua.UserTokenTypeAnonymous),
	)
	ns := server.NewNodeNameSpace(s, "urn:industrial:test")
	v := ns.AddNewVariableNode("DemoVar", int32(42))
	ns.Objects().AddRef(v, id.HasComponent, true)
	if root, err := s.Namespace(0); err == nil {
		root.Objects().AddRef(ns.Objects(), id.HasComponent, true)
	}
	if err := s.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return fmt.Sprintf("127.0.0.1:%d", port), v.ID().String(), ns.Objects().ID().String()
}

func run(t *testing.T, req apidoc.DebugRequest) *apidoc.DebugResponse {
	t.Helper()
	resp, err := New().Exec(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func TestRead(t *testing.T) {
	addr, nodeID, _ := startOPCUA(t)
	resp := run(t, apidoc.DebugRequest{
		Method: "READ", URL: "opcua://" + addr,
		Params:  []apidoc.DebugValue{{Name: "node_id", Value: nodeID}},
		Options: map[string]string{"timeout_ms": "5000"},
	})
	var out struct {
		Type  string `json:"type"`
		Value any    `json:"value"`
	}
	if err := json.Unmarshal(resp.Body, &out); err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(out.Value) != "42" {
		t.Fatalf("value = %v (type %s)", out.Value, out.Type)
	}
}

func TestWriteThenRead(t *testing.T) {
	addr, nodeID, _ := startOPCUA(t)
	run(t, apidoc.DebugRequest{
		Method: "WRITE", URL: "opcua://" + addr,
		Params: []apidoc.DebugValue{
			{Name: "node_id", Value: nodeID}, {Name: "value", Value: "99"},
		},
		Options: map[string]string{"value_type": "int32", "timeout_ms": "5000"},
	})
	resp := run(t, apidoc.DebugRequest{
		Method: "READ", URL: "opcua://" + addr,
		Params:  []apidoc.DebugValue{{Name: "node_id", Value: nodeID}},
		Options: map[string]string{"timeout_ms": "5000"},
	})
	var out struct {
		Value any `json:"value"`
	}
	if err := json.Unmarshal(resp.Body, &out); err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(out.Value) != "99" {
		t.Fatalf("value after write = %v", out.Value)
	}
}

func TestBrowse(t *testing.T) {
	addr, nodeID, objectsID := startOPCUA(t)
	resp := run(t, apidoc.DebugRequest{
		Method: "BROWSE", URL: "opcua://" + addr,
		Params:  []apidoc.DebugValue{{Name: "node_id", Value: objectsID}},
		Options: map[string]string{"timeout_ms": "5000"},
	})
	var out struct {
		Children []string `json:"children"`
	}
	if err := json.Unmarshal(resp.Body, &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Children) != 1 || out.Children[0] != nodeID {
		t.Fatalf("children = %v, want [%s]", out.Children, nodeID)
	}
}

func TestBadURL(t *testing.T) {
	if _, err := New().Exec(context.Background(), apidoc.DebugRequest{
		Method: "READ", URL: "http://127.0.0.1:1", Params: []apidoc.DebugValue{{Name: "node_id", Value: "i=85"}},
	}); err == nil {
		t.Fatal("want error for non-opcua url")
	}
}

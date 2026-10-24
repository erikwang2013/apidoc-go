// Package opcua debugs OPC UA servers with gopcua: read a node value, write
// a value, or browse a node's children.
package opcua

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/erikwang2013/apidoc-go"
	"github.com/erikwang2013/apidoc-go/example/debugger-industrial/internal/opts"
	"github.com/gopcua/opcua"
	"github.com/gopcua/opcua/id"
	"github.com/gopcua/opcua/ua"
)

// New returns the OPC UA debugger. The URL is opcua://host:port (opc.tcp://
// is also accepted); the node is taken from the node_id parameter, the ?node=
// query, or the URL path.
func New() apidoc.Debugger {
	return apidoc.DebuggerFunc("opcua", apidoc.DebugSpec{
		Mode:         "server",
		Actions:      []string{"READ", "WRITE", "BROWSE"},
		Encodings:    []string{"text", "json"},
		ResponseView: "json",
		ConnectFields: []apidoc.DebugField{
			{Name: "value_type", Label: "Write value type", Type: "select", Default: "auto",
				Options: []string{"auto", "int32", "int64", "uint32", "uint64", "float32", "float64", "bool", "string"}},
			{Name: "username", Label: "Username (empty = anonymous)", Type: "text"},
			{Name: "password", Label: "Password", Type: "password", Secret: true},
			{Name: "timeout_ms", Label: "Timeout (ms)", Type: "number", Default: "10000"},
		},
	}, exec)
}

func exec(ctx context.Context, req apidoc.DebugRequest) (*apidoc.DebugResponse, error) {
	u, err := url.Parse(req.URL)
	if err != nil || (u.Scheme != "opcua" && u.Scheme != "opc.tcp") || u.Host == "" {
		return nil, fmt.Errorf("opcua: url must be opcua://host:port")
	}
	nodeID := nodeFrom(req, u)
	if nodeID == "" {
		return nil, fmt.Errorf("opcua: node_id is required")
	}
	timeout, err := opts.Timeout(req.Options, "timeout_ms", 10*time.Second, 60*time.Second)
	if err != nil {
		return nil, err
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	copts := []opcua.Option{}
	if user := req.Options["username"]; user != "" {
		copts = append(copts, opcua.AuthUsername(user, req.Options["password"]))
	}
	c, err := opcua.NewClient("opc.tcp://"+u.Host, copts...)
	if err != nil {
		return nil, err
	}
	if err := c.Connect(cctx); err != nil {
		return nil, err
	}
	defer c.Close(cctx)

	nid, err := ua.ParseNodeID(nodeID)
	if err != nil {
		return nil, fmt.Errorf("opcua: node_id %q: %w", nodeID, err)
	}
	node := c.Node(nid)

	var out map[string]any
	switch req.Method {
	case "READ":
		v, err := node.Value(cctx)
		if err != nil {
			return nil, err
		}
		out = map[string]any{"node_id": nodeID, "type": fmt.Sprint(v.Type()), "value": jsonValue(v.Value())}
	case "WRITE":
		v, err := parseVariant(param(req, "value"), req.Options["value_type"])
		if err != nil {
			return nil, err
		}
		resp, err := c.Write(cctx, &ua.WriteRequest{NodesToWrite: []*ua.WriteValue{{
			NodeID: nid, AttributeID: ua.AttributeIDValue,
			Value: &ua.DataValue{EncodingMask: ua.DataValueValue, Value: v},
		}}})
		if err != nil {
			return nil, err
		}
		status := resp.Results[0]
		if status != ua.StatusOK {
			return nil, fmt.Errorf("opcua: write status %s", status)
		}
		out = map[string]any{"node_id": nodeID, "status": status.Error(), "written": jsonValue(v.Value())}
	case "BROWSE":
		children, err := node.Children(cctx, id.HierarchicalReferences, ua.NodeClassAll)
		if err != nil {
			return nil, err
		}
		ids := make([]string, len(children))
		for i, ch := range children {
			ids[i] = ch.ID.String()
		}
		out = map[string]any{"node_id": nodeID, "children": ids}
	default:
		return nil, fmt.Errorf("opcua: unknown action %q", req.Method)
	}
	body, _ := json.Marshal(out)
	return &apidoc.DebugResponse{
		Status:   "OK",
		Body:     body,
		Encoding: "json",
		Headers:  map[string]string{"endpoint": "opc.tcp://" + u.Host},
	}, nil
}

func nodeFrom(req apidoc.DebugRequest, u *url.URL) string {
	if v := param(req, "node_id"); v != "" {
		return v
	}
	if v := u.Query().Get("node"); v != "" {
		return v
	}
	return strings.TrimPrefix(u.Path, "/")
}

func param(req apidoc.DebugRequest, name string) string {
	for _, p := range req.Params {
		if p.Name == name && p.Value != "" {
			return p.Value
		}
	}
	return req.Options[name]
}

// parseVariant types the panel string so it matches the node's data type.
// auto infers bool, int32 or float64 and otherwise falls back to string.
func parseVariant(s, typ string) (*ua.Variant, error) {
	if typ == "" || typ == "auto" {
		if b, err := strconv.ParseBool(s); err == nil {
			return ua.NewVariant(b)
		}
		if i, err := strconv.ParseInt(s, 10, 32); err == nil {
			return ua.NewVariant(int32(i))
		}
		if f, err := strconv.ParseFloat(s, 64); err == nil {
			return ua.NewVariant(f)
		}
		return ua.NewVariant(s)
	}
	var v any
	var err error
	switch typ {
	case "int32":
		var i int64
		i, err = strconv.ParseInt(s, 0, 32)
		v = int32(i)
	case "int64":
		v, err = strconv.ParseInt(s, 0, 64)
	case "uint32":
		var i uint64
		i, err = strconv.ParseUint(s, 0, 32)
		v = uint32(i)
	case "uint64":
		v, err = strconv.ParseUint(s, 0, 64)
	case "float32":
		var f float64
		f, err = strconv.ParseFloat(s, 32)
		v = float32(f)
	case "float64":
		v, err = strconv.ParseFloat(s, 64)
	case "bool":
		v, err = strconv.ParseBool(s)
	case "string":
		v = s
	default:
		return nil, fmt.Errorf("opcua: unknown value_type %q", typ)
	}
	if err != nil {
		return nil, fmt.Errorf("opcua: value: %w", err)
	}
	return ua.NewVariant(v)
}

func jsonValue(v any) any {
	switch x := v.(type) {
	case []byte:
		return hex.EncodeToString(x)
	case time.Time:
		return x.Format(time.RFC3339Nano)
	default:
		return x
	}
}

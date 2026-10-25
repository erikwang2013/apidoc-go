package parse_test

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/erikwang2013/apidoc-go/model"
	"github.com/erikwang2013/apidoc-go/parse"
)

func src(lines ...string) string { return strings.Join(lines, "\n") }

func TestTags(t *testing.T) {
	res, err := parse.ParseSource("tags.go", []byte(src(
		"package p",
		"",
		"// @apidoc",
		"// @method POST",
		"// @url /api/v1/users",
		"// @title 创建用户",
		"// @desc creates a user",
		"// @author erik",
		"// @app demo",
		"// @version v1",
		"// @group user",
		"// @sort 3",
		"// @markdown first line",
		"// @markdown second line",
		"func CreateUser() {}",
	)))
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 1 {
		t.Fatalf("want 1 result, got %d", len(res))
	}
	r := res[0]
	if r.Method != "POST" || r.URL != "/api/v1/users" {
		t.Errorf("method/url mismatch: %+v", r)
	}
	d := r.Doc
	if d.Title != "创建用户" || d.Desc != "creates a user" || d.Author != "erik" ||
		d.App != "demo" || d.Version != "v1" || d.Controller != "user" || d.Sort != 3 {
		t.Errorf("tags mismatch: %+v", d)
	}
	if d.Markdown != "first line\nsecond line" {
		t.Errorf("markdown mismatch: %q", d.Markdown)
	}
}

func TestParams(t *testing.T) {
	res, err := parse.ParseSource("params.go", []byte(src(
		"package p",
		"type Filter struct { Kind string `json:\"kind\"` }",
		"// @apidoc",
		"// @param id int true \"the id\"",
		"// @param q string false search in=query default=hello mock=world",
		"// @param filter object false filter children=Filter",
		"// @header X-Token string true \"auth token\"",
		"// @body data object false payload",
		"func Search() {}",
	)))
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 1 {
		t.Fatalf("want 1 result, got %d", len(res))
	}
	want := []model.Param{
		{Name: "id", In: "query", Type: "int", Required: true, Desc: `"the id"`},
		{Name: "q", In: "query", Type: "string", Required: false, Desc: "search", Default: "hello", Mock: "world"},
		{Name: "filter", In: "query", Type: "Filter", Desc: "filter",
			Children: []model.Param{{Name: "kind", Type: "string"}}},
		{Name: "X-Token", In: "header", Type: "string", Required: true, Desc: `"auth token"`},
		{Name: "data", In: "body", Type: "object", Desc: "payload"},
	}
	if !reflect.DeepEqual(res[0].Doc.Params, want) {
		t.Errorf("params mismatch:\n got %+v\nwant %+v", res[0].Doc.Params, want)
	}
}

func TestResponses(t *testing.T) {
	res, err := parse.ParseSource("resp.go", []byte(src(
		"package p",
		"// @apidoc",
		"// @success ok User \"user info\"",
		"// @success User",
		"// @success {list}User",
		"// @response ok string",
		"func Get() {}",
	)))
	if err != nil {
		t.Fatal(err)
	}
	want := []model.Response{
		{Name: "ok", Type: "User", Desc: `"user info"`},
		{Name: "ok", Type: "User"},
		{Name: "ok", Type: "[]User"},
		{Name: "ok", Type: "string"},
	}
	if !reflect.DeepEqual(res[0].Doc.Responses, want) {
		t.Errorf("responses mismatch:\n got %+v\nwant %+v", res[0].Doc.Responses, want)
	}
}

func TestStructs(t *testing.T) {
	res, err := parse.ParseSource("structs.go", []byte(src(
		"package p",
		"type User struct {",
		"	ID    int    `json:\"id\"`",
		"	Name  string `json:\"name,omitempty\"`",
		"	Age   int",
		"	pass  string",
		"	Skip  string `json:\"-\"`",
		"}",
		"// @apidoc",
		"func CreateUser(c *gin.Context, u User, us []User, n int) (*User, error) {}",
	)))
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 1 {
		t.Fatalf("want 1 result, got %d", len(res))
	}
	wantParams := []model.Param{
		{Name: "u", In: "body", Type: "User", Children: []model.Param{
			{Name: "id", Type: "int"},
			{Name: "name", Type: "string"},
			{Name: "Age", Type: "int"},
		}},
		{Name: "us", In: "body", Type: "User", Children: []model.Param{
			{Name: "id", Type: "int"},
			{Name: "name", Type: "string"},
			{Name: "Age", Type: "int"},
		}},
	}
	wantResp := []model.Response{
		{Name: "ok", Type: "User", Fields: []model.Param{
			{Name: "id", Type: "int"},
			{Name: "name", Type: "string"},
			{Name: "Age", Type: "int"},
		}},
	}
	d := res[0].Doc
	if !reflect.DeepEqual(d.Params, wantParams) {
		t.Errorf("struct params mismatch:\n got %+v\nwant %+v", d.Params, wantParams)
	}
	if !reflect.DeepEqual(d.Responses, wantResp) {
		t.Errorf("struct responses mismatch:\n got %+v\nwant %+v", d.Responses, wantResp)
	}
}

func TestMalformed(t *testing.T) {
	cases := []struct {
		name, line string
	}{
		{"param too few", "// @param id"},
		{"empty in", "// @param id string in="},
		{"unknown key", "// @param id string foo=1"},
		{"bad sort", "// @sort x"},
		{"sort wants value", "// @sort"},
		{"method wants value", "// @method"},
		{"response no type", "// @success"},
		{"response unknown key", "// @success ok User a=1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if p := recover(); p != nil {
					t.Fatalf("panic: %v", p)
				}
			}()
			_, err := parse.ParseSource("bad.go", []byte(src(
				"package p",
				"// @apidoc",
				tc.line,
				"func F() {}",
			)))
			if err == nil {
				t.Fatal("want error, got nil")
			}
			if !strings.Contains(err.Error(), "bad.go:") {
				t.Errorf("error missing source position: %v", err)
			}
			if !strings.Contains(err.Error(), "F") {
				t.Errorf("error missing func name: %v", err)
			}
		})
	}
	// malformed Go source must error, not panic
	if _, err := parse.ParseSource("broken.go", []byte("package p\nfunc broken(")); err == nil {
		t.Fatal("want syntax error, got nil")
	}
}

func TestDefaults(t *testing.T) {
	res, err := parse.ParseSource("defaults.go", []byte(src(
		"package p",
		"// @apidoc",
		"func Ping() {}",
		"",
		"// no marker here",
		"func Unannotated() {}",
	)))
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 1 {
		t.Fatalf("unannotated func not skipped: %d results", len(res))
	}
	if res[0].Doc.Controller != "Ping" || res[0].Doc.Title != "Ping" {
		t.Errorf("defaults mismatch: %+v", res[0].Doc)
	}
}

func TestDeterminism(t *testing.T) {
	srcText := []byte(src(
		"package p",
		"type User struct { ID int `json:\"id\"` }",
		"// @apidoc",
		"// @param id int true \"the id\"",
		"// @success ok User \"user info\"",
		"func F(u User) User {}",
	))
	a, err := parse.ParseSource("d1.go", srcText)
	if err != nil {
		t.Fatal(err)
	}
	b, err := parse.ParseSource("d2.go", srcText)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(a, b) {
		t.Errorf("nondeterministic parse:\n a %+v\n b %+v", a, b)
	}
}

func TestParseDirAndFile(t *testing.T) {
	dir := t.TempDir()
	good := filepath.Join(dir, "good.go")
	if err := os.WriteFile(good, []byte(src(
		"package p",
		"// @apidoc",
		"// @method GET",
		"func Good() {}",
	)), 0o644); err != nil {
		t.Fatal(err)
	}
	// _test.go files are skipped even when annotated
	if err := os.WriteFile(filepath.Join(dir, "skip_test.go"), []byte(src(
		"package p",
		"// @apidoc",
		"func SkipMe() {}",
	)), 0o644); err != nil {
		t.Fatal(err)
	}
	bad := filepath.Join(dir, "bad.go")
	if err := os.WriteFile(bad, []byte(src(
		"package p",
		"// @apidoc",
		"// @param broken",
		"func Broken() {}",
	)), 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := parse.ParseDir(dir)
	if len(res) != 1 || res[0].Doc.Controller != "Good" {
		t.Errorf("ParseDir results: %+v", res)
	}
	if err == nil || !strings.Contains(err.Error(), "bad.go") {
		t.Errorf("ParseDir should aggregate bad.go error, got %v", err)
	}

	rs, err := parse.ParseFile(good)
	if err != nil || len(rs) != 1 || rs[0].Method != "GET" {
		t.Errorf("ParseFile: rs=%+v err=%v", rs, err)
	}
}

func TestProtocolTagAndCustomIn(t *testing.T) {
	res, err := parse.ParseSource("proto.go", []byte(src(
		"package p",
		"// @apidoc",
		"// @protocol MQTT",
		"// @method PUBLISH",
		"// @url mqtt://broker:1883/sensors/temp",
		"// @param qos int false \"QoS\" in=option default=1",
		"// @param payload json true \"消息体\" in=payload",
		"func Publish() {}",
	)))
	if err != nil {
		t.Fatal(err)
	}
	r := res[0]
	if r.Doc.Protocol != "mqtt" {
		t.Fatalf("protocol = %q", r.Doc.Protocol)
	}
	if r.Doc.Params[0].In != "option" || r.Doc.Params[1].In != "payload" {
		t.Fatalf("custom in groups lost: %+v", r.Doc.Params)
	}
}

func TestProtocolInferredFromURL(t *testing.T) {
	res, err := parse.ParseSource("infer.go", []byte(src(
		"package p",
		"// @apidoc",
		"// @method CALL",
		"// @url grpc://localhost:50051/pkg.Svc/M",
		"func Call() {}",
	)))
	if err != nil || len(res) != 1 {
		t.Fatalf("parse: %v %d", err, len(res))
	}
	if res[0].Doc.Protocol != "grpc" {
		t.Fatalf("inferred = %q", res[0].Doc.Protocol)
	}
	res, err = parse.ParseSource("http.go", []byte(src(
		"package p",
		"// @apidoc",
		"// @method GET",
		"// @url /api/x",
		"func Get() {}",
	)))
	if err != nil || res[0].Doc.Protocol != "http" {
		t.Fatalf("http infer: %v %+v", err, res)
	}
}

func TestProtocolAliasesAndMultiword(t *testing.T) {
	res, err := parse.ParseSource("aliases.go", []byte(src(
		"package p",
		"// @apidoc",
		"// @protocol ISO 8583",
		"// @method SEND",
		"// @url iso8583://host:9000/0100",
		"func A() {}",
		"// @apidoc",
		"// @protocol OPC UA",
		"// @method READ",
		"// @url opc.tcp://host:4840",
		"func B() {}",
		"// @apidoc",
		"// @protocol WebRTC DataChannel",
		"// @method SEND",
		"// @url webrtc://host",
		"func C() {}",
		"// @apidoc",
		"// @protocol REST",
		"// @method GET",
		"// @url https://host/x",
		"func D() {}",
		"// @apidoc",
		"// @protocol JSON-RPC",
		"// @method CALL",
		"// @url json-rpc://host",
		"func E() {}",
	)))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"iso8583", "opcua", "webrtc", "http", "jsonrpc"}
	if len(res) != len(want) {
		t.Fatalf("got %d results, want %d", len(res), len(want))
	}
	for i := range want {
		if res[i].Doc.Protocol != want[i] {
			t.Errorf("result %d protocol = %q, want %q", i, res[i].Doc.Protocol, want[i])
		}
	}
}

func TestProtocolOptsAndEvents(t *testing.T) {
	res, err := parse.ParseSource("events.go", []byte(src(
		"package p",
		"// @apidoc",
		"// @protocol mqtt",
		"// @method SUBSCRIBE",
		"// @url mqtt://broker:1883/sensors/+",
		"// @protocol-opt target=mqtt://broker:1883",
		"// @protocol-opt topic=sensors/+",
		`// @event direction=in name=temperature type=json desc=temp mock={"v":1}`,
		"func Subscribe() {}",
	)))
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 1 {
		t.Fatalf("want 1 result, got %d", len(res))
	}
	d := res[0].Doc
	if d.ProtocolOpts["target"] != "mqtt://broker:1883" || d.ProtocolOpts["topic"] != "sensors/+" {
		t.Fatalf("protocol opts mismatch: %+v", d.ProtocolOpts)
	}
	if len(d.Events) != 1 || d.Events[0].Direction != "in" ||
		d.Events[0].Name != "temperature" || d.Events[0].Type != "json" ||
		d.Events[0].Desc != "temp" || d.Events[0].Mock != `{"v":1}` {
		t.Fatalf("events mismatch: %+v", d.Events)
	}
}

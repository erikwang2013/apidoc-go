package protocol

import (
	"strings"
	"testing"
)

type mapResolver map[string]string

func (m mapResolver) Read(path string) (string, string, bool) {
	content, ok := m[path]
	return path, content, ok
}

func (m mapResolver) Failure() string { return "" }

func parseFixture(t *testing.T, path, format string, options Options) Definition {
	t.Helper()
	return Parse(format, path, readFixture(t, path), options, DefaultLimits(), &fixtureResolver{})
}

func withOperation(operation string) Options { return Options{Operation: operation} }

func messageFields(t *testing.T, message map[string]any) []map[string]any {
	t.Helper()
	fields, ok := message["fields"].([]map[string]any)
	if !ok {
		t.Fatalf("fields: %#v", message["fields"])
	}
	return fields
}

func field(t *testing.T, message map[string]any, index int) map[string]any {
	t.Helper()
	fields := messageFields(t, message)
	if index >= len(fields) {
		t.Fatalf("field %d missing in %#v", index, fields)
	}
	return fields[index]
}

func assertError(t *testing.T, validation Validation, message string) {
	t.Helper()
	if validation.OK {
		t.Fatalf("expected error %q, validation passed", message)
	}
	if len(validation.Errors) == 0 {
		t.Fatalf("expected error %q, got none", message)
	}
	error := validation.Errors[0]
	if error.Path != "" || error.Severity != "error" || error.Message != message {
		t.Fatalf("unexpected diagnostic: %#v", error)
	}
}

func TestDetectFormats(t *testing.T) {
	cases := []struct {
		path, content, want string
	}{
		{"x.proto", "", "proto"},
		{"x.thrift", "", "thrift"},
		{"x.graphql", "", "graphql"},
		{"x.gql", "", "graphql"},
		{"x.wsdl", "", "wsdl"},
		{"x.yaml", "openapi: 3.0.0", "openapi"},
		{"x.yaml", `"swagger": "2.0"`, "openapi"},
		{"x.json", `"asyncapi": "2.6.0"`, "asyncapi"},
		{"x.json", `"methods": {}`, "json-rpc"},
		{"x.json", `"$schema": "x"`, "json-schema"},
		{"x.json", `"$ref": "#/x"`, "json-schema"},
		{"x.json", `"properties": {}`, "json-schema"},
		{"x.xml", `<edmx:Edmx/>`, "edmx"},
		{"x.xml", `<definitions xmlns="wsdl"/>`, "wsdl"},
		{"x.xml", "<methodCall/>", "xml-rpc"},
		{"x.xml", "<methodResponse/>", "xml-rpc"},
		{"x.xml", "<Envelope/>", "wsdl"},
		{"dir/a.JSON", `"type": "object"`, "json-schema"},
		{"x.txt", "", ""},
		{"x", "", ""},
	}
	for _, c := range cases {
		if got := Detect(c.path, c.content); got != c.want {
			t.Errorf("Detect(%q, %q) = %q, want %q", c.path, c.content, got, c.want)
		}
	}
}

func TestOpenAPISuccessAndError(t *testing.T) {
	definition := parseFixture(t, "openapi/api.yaml", "openapi", withOperation("getUser"))
	if definition.Operation != "getUser" || !definition.Validation.OK {
		t.Fatalf("unexpected definition: %#v", definition)
	}
	if got := field(t, definition.Messages[0], 0); got["name"] != "id" || got["type"] != "integer" || got["require"] != true {
		t.Fatalf("request field: %#v", got)
	}
	if definition.Messages[1]["name"] != "Response 200" {
		t.Fatalf("response name: %#v", definition.Messages[1]["name"])
	}
	if got := field(t, definition.Messages[1], 1); got["name"] != "name" {
		t.Fatalf("response field: %#v", got)
	}
	bad := parseFixture(t, "openapi/api.yaml", "openapi", withOperation("nope"))
	assertError(t, bad.Validation, "operation not found: nope; candidates: getUser, createUser")
}

func TestAsyncAPISuccessAndError(t *testing.T) {
	definition := parseFixture(t, "asyncapi/asyncapi.yaml", "asyncapi", withOperation("sensors/+/temp:publish"))
	if definition.Operation != "sensors/+/temp:publish" || !definition.Validation.OK {
		t.Fatalf("unexpected definition: %#v", definition)
	}
	if definition.Messages[0]["name"] != "Temperature" || definition.Messages[0]["direction"] != "event" {
		t.Fatalf("message: %#v", definition.Messages[0])
	}
	if got := field(t, definition.Messages[0], 0); got["type"] != "number" {
		t.Fatalf("first field: %#v", got)
	}
	if got := field(t, definition.Messages[0], 1); got["type"] != "string" {
		t.Fatalf("second field: %#v", got)
	}
	bad := parseFixture(t, "asyncapi/asyncapi.yaml", "asyncapi", withOperation("nope"))
	assertError(t, bad.Validation, "operation not found: nope; candidates: sensors/+/temp:publish")
}

func TestGraphQLSuccessAndError(t *testing.T) {
	definition := parseFixture(t, "graphql/schema.graphql", "graphql", withOperation("Query.user"))
	if definition.Operation != "Query.user" || !definition.Validation.OK {
		t.Fatalf("unexpected definition: %#v", definition)
	}
	if got := field(t, definition.Messages[0], 0); got["require"] != true {
		t.Fatalf("request field: %#v", got)
	}
	if got := field(t, definition.Messages[1], 2); got["type"] != "array" || got["childrenType"] != "string" {
		t.Fatalf("response field: %#v", got)
	}
	bad := parseFixture(t, "graphql/schema.graphql", "graphql", withOperation("Query.nope"))
	assertError(t, bad.Validation, "operation not found: Query.nope; candidates: Query.user")
}

func TestProtoSuccessAndError(t *testing.T) {
	definition := parseFixture(t, "proto/helloworld.proto", "proto", withOperation("helloworld.Greeter/SayHello"))
	if definition.Operation != "helloworld.Greeter/SayHello" || !definition.Validation.OK {
		t.Fatalf("unexpected definition: %#v", definition)
	}
	if definition.Meta["package"] != "helloworld" {
		t.Fatalf("meta: %#v", definition.Meta)
	}
	if got := field(t, definition.Messages[0], 0); got["fieldNumber"] != int64(1) {
		t.Fatalf("fieldNumber: %#v", got)
	}
	children, _ := field(t, definition.Messages[0], 1)["children"].([]map[string]any)
	if len(children) == 0 || children[0]["name"] != "trace_id" {
		t.Fatalf("nested children: %#v", field(t, definition.Messages[0], 1))
	}
	if got := field(t, definition.Messages[0], 2); got["type"] != "array" {
		t.Fatalf("repeated field: %#v", got)
	}

	bad := parseFixture(t, "proto/unsupported.proto", "proto", Options{})
	if bad.Validation.OK || len(bad.Validation.Errors) != 2 {
		t.Fatalf("errors: %#v", bad.Validation.Errors)
	}
	first := bad.Validation.Errors[0]
	if first.Line != 2 || first.Column != 1 || first.Message != "unsupported construct: extend" {
		t.Fatalf("first diagnostic: %#v", first)
	}
	if got := bad.Validation.Errors[1].Message; got != "operation required; candidates: " {
		t.Fatalf("second diagnostic: %#v", bad.Validation.Errors[1])
	}
}

func TestWSDLSuccessAndError(t *testing.T) {
	definition := parseFixture(t, "wsdl/calculator.wsdl", "wsdl", withOperation("CalculatorService/CalculatorPort/Add"))
	if definition.Operation != "CalculatorService/CalculatorPort/Add" || !definition.Validation.OK {
		t.Fatalf("unexpected definition: %#v", definition)
	}
	if definition.Meta["service"] != "CalculatorService" || definition.Meta["port"] != "CalculatorPort" {
		t.Fatalf("meta: %#v", definition.Meta)
	}
	if got := field(t, definition.Messages[0], 0); got["type"] != "integer" || got["typeName"] != "int" {
		t.Fatalf("request field: %#v", got)
	}
	if got := field(t, definition.Messages[1], 0); got["name"] != "result" {
		t.Fatalf("response field: %#v", got)
	}
	bad := parseFixture(t, "wsdl/calculator.wsdl", "wsdl", withOperation("Nope"))
	assertError(t, bad.Validation, "operation not found: Nope; candidates: CalculatorService/CalculatorPort/Add")
}

func TestEDMXSuccessAndError(t *testing.T) {
	definition := parseFixture(t, "edmx/metadata.xml", "edmx", withOperation("Products"))
	if definition.Operation != "Products" || !definition.Validation.OK || definition.Meta["direction"] != "response" {
		t.Fatalf("unexpected definition: %#v", definition)
	}
	if got := field(t, definition.Messages[0], 0); got["require"] != true {
		t.Fatalf("field: %#v", got)
	}
	if got := field(t, definition.Messages[0], 2); got["type"] != "number" {
		t.Fatalf("field: %#v", got)
	}
	bad := parseFixture(t, "edmx/metadata.xml", "edmx", withOperation("Nope"))
	assertError(t, bad.Validation, "operation not found: Nope; candidates: Products")
}

func TestJSONSchemaSuccessAndError(t *testing.T) {
	definition := parseFixture(t, "json-schema/user.json", "json-schema", Options{
		Sample: `{"id":1,"name":"a"}`,
	})
	if definition.Messages[0]["name"] != "User" {
		t.Fatalf("message: %#v", definition.Messages[0])
	}
	sample := definition.Messages[0]["sampleValidation"].(SampleValidation)
	if !sample.OK {
		t.Fatalf("sample: %#v", sample)
	}
	if got := field(t, definition.Messages[0], 0); got["require"] != true {
		t.Fatalf("field: %#v", got)
	}
	if got := field(t, definition.Messages[0], 2); got["type"] != "array" {
		t.Fatalf("field: %#v", got)
	}

	bad := parseFixture(t, "json-schema/invalid.json", "json-schema", Options{})
	assertError(t, bad.Validation, "JSON parse error: Syntax error")
	if len(bad.Messages) != 0 {
		t.Fatalf("messages: %#v", bad.Messages)
	}
}

func TestJSONRPCSuccessAndError(t *testing.T) {
	definition := parseFixture(t, "json-rpc/methods.json", "json-rpc", Options{
		Operation: "user.get",
		Sample:    `{"jsonrpc":"2.0","method":"user.get","params":{"id":1},"id":1}`,
	})
	if definition.Operation != "user.get" {
		t.Fatalf("operation: %q", definition.Operation)
	}
	if sample := definition.Messages[0]["sampleValidation"].(SampleValidation); !sample.OK {
		t.Fatalf("sample: %#v", sample)
	}
	if got := field(t, definition.Messages[1], 1); got["name"] != "name" {
		t.Fatalf("response field: %#v", got)
	}

	bad := parseFixture(t, "json-rpc/methods.json", "json-rpc", Options{
		Operation: "user.get",
		Sample:    `{"jsonrpc":"2.0","method":"user.get","params":{},"id":1}`,
	})
	sample := bad.Messages[0]["sampleValidation"].(SampleValidation)
	if sample.OK || len(sample.Errors) == 0 {
		t.Fatalf("sample: %#v", sample)
	}
	if sample.Errors[0].Path != "$.id" || sample.Errors[0].Severity != "error" || sample.Errors[0].Message != "is required" {
		t.Fatalf("sample error: %#v", sample.Errors[0])
	}
}

func TestXMLRPCSuccessAndError(t *testing.T) {
	definition := parseFixture(t, "xml-rpc/methods.xml", "xml-rpc", withOperation("add"))
	if definition.Operation != "add" || !definition.Validation.OK {
		t.Fatalf("unexpected definition: %#v", definition)
	}
	if got := field(t, definition.Messages[0], 0); got["name"] != "a" {
		t.Fatalf("request field: %#v", got)
	}
	if got := field(t, definition.Messages[1], 0); got["type"] != "integer" {
		t.Fatalf("response field: %#v", got)
	}
	badSample := parseFixture(t, "xml-rpc/methods.xml", "xml-rpc", Options{
		Operation: "add",
		Sample:    "<methodCall><methodName>other</methodName></methodCall>",
	})
	sample := badSample.Messages[0]["sampleValidation"].(SampleValidation)
	if sample.OK || len(sample.Errors) == 0 || sample.Errors[0].Message != "methodName mismatch" {
		t.Fatalf("sample: %#v", sample)
	}
	badOperation := parseFixture(t, "xml-rpc/methods.xml", "xml-rpc", withOperation("nope"))
	assertError(t, badOperation.Validation, "operation not found: nope; candidates: add")
}

func TestUnknownFormatHasStableError(t *testing.T) {
	definition := Parse("unknown", "x.unknown", "", Options{}, DefaultLimits(), &fixtureResolver{})
	if definition.Format != "unknown" {
		t.Fatalf("format: %q", definition.Format)
	}
	assertError(t, definition.Validation, "unsupported definition format: unknown")
	if len(definition.Messages) != 0 {
		t.Fatalf("messages: %#v", definition.Messages)
	}
}

func TestSizeAndLineLimitsAreEnforced(t *testing.T) {
	content := "{\n\"type\":\"object\"\n}"
	byBytes := Parse("json-schema", "x.json", content, Options{}, Limits{MaxFileBytes: 1}, &fixtureResolver{})
	assertError(t, byBytes.Validation, "file exceeds max_file_bytes")
	byLines := Parse("json-schema", "x.json", content, Options{}, Limits{MaxLines: 1}, &fixtureResolver{})
	assertError(t, byLines.Validation, "file exceeds max_lines")
}

func TestXMLDoctypeIsRejected(t *testing.T) {
	content := `<!DOCTYPE foo [<!ENTITY xxe SYSTEM "file:///etc/passwd">]><edmx:Edmx/>`
	definition := Parse("edmx", "x.xml", content, Options{}, DefaultLimits(), &fixtureResolver{})
	assertError(t, definition.Validation, "DOCTYPE is not allowed")
}

func TestUnreadableImportsReportTheReadFailure(t *testing.T) {
	resolver := &failureResolver{}
	definition := Parse("json-schema", "x.json", `{"$ref":"big.json#/X"}`, Options{}, DefaultLimits(), resolver)
	messages := diagnosticMessages(definition.Validation.Errors)
	if !containsString(messages, "unable to read file") || !containsString(messages, "unable to parse $ref: big.json#/X") {
		t.Fatalf("errors: %#v", messages)
	}
	for _, message := range messages {
		if strings.HasPrefix(message, "unable to resolve $ref:") {
			t.Fatalf("unexpected resolve error: %#v", messages)
		}
	}
}

type failureResolver struct{}

func (*failureResolver) Read(string) (string, string, bool) { return "", "", false }
func (*failureResolver) Failure() string                    { return "unable to read file" }

type inlineResolver struct{ content string }

func (r *inlineResolver) Read(path string) (string, string, bool) {
	return path, r.content, true
}

func (r *inlineResolver) Failure() string { return "" }

func TestImportedFilesObeyByteAndLineLimits(t *testing.T) {
	root := `{"$ref":"big.json#"}`
	imported := `{"X":{"type":"object","properties":{"a":{"type":"string"}}}}`
	byBytes := Parse("json-schema", "x.json", root, Options{}, Limits{MaxFileBytes: 32}, &inlineResolver{content: imported})
	assertError(t, byBytes.Validation, "file exceeds max_file_bytes")
	byLines := Parse("json-schema", "x.json", root, Options{}, Limits{MaxLines: 1}, &inlineResolver{content: "{\n}\n"})
	assertError(t, byLines.Validation, "file exceeds max_lines")
}

func TestYAMLSecurityLimitsAreRejected(t *testing.T) {
	cases := []struct{ content, message string }{
		{"openapi: 3.0.0\ninfo: &x {title: x}\npaths: {}\n", "YAML: anchors, aliases and tags are not supported"},
		{"openapi: 3.0.0\n\tinfo: {title: x}\npaths: {}\n", "YAML: tab indentation is not supported"},
		{"openapi: 3.0.0\n---\npaths: {}\n", "YAML: multi-document YAML is not supported"},
	}
	for _, c := range cases {
		definition := Parse("openapi", "x.yaml", c.content, Options{}, DefaultLimits(), &fixtureResolver{})
		assertError(t, definition.Validation, c.message)
	}
}

func TestDeeplyNestedDocumentsAreRejected(t *testing.T) {
	yaml := "openapi: 3.0.0\ninfo: {title: x, version: \"1\"}\npaths: {}\nx: " +
		strings.Repeat("[", 300) + strings.Repeat("]", 300) + "\n"
	yamlDefinition := Parse("openapi", "x.yaml", yaml, Options{}, DefaultLimits(), &fixtureResolver{})
	assertError(t, yamlDefinition.Validation, "YAML: nesting limit reached")

	json := strings.Repeat("[", 600) + strings.Repeat("]", 600)
	jsonDefinition := Parse("json-schema", "x.json", json, Options{}, DefaultLimits(), &fixtureResolver{})
	if jsonDefinition.Validation.OK || !strings.HasPrefix(jsonDefinition.Validation.Errors[0].Message, "JSON parse error:") {
		t.Fatalf("errors: %#v", jsonDefinition.Validation.Errors)
	}

	xml := `<edmx:Edmx xmlns:edmx="http://docs.oasis-open.org/odata/ns/edmx">` +
		strings.Repeat("<a>", 300) + strings.Repeat("</a>", 300) + "</edmx:Edmx>"
	xmlDefinition := Parse("edmx", "x.xml", xml, Options{}, DefaultLimits(), &fixtureResolver{})
	assertError(t, xmlDefinition.Validation, "XML parse error: XML nesting limit exceeded")
}

func TestLoneFlowCollectionsDoNotPanic(t *testing.T) {
	for _, content := range []string{
		"openapi: 3.0.0\ninfo: {title: x, version: \"1\"}\npaths: {}\ntype: [\n",
		"openapi: 3.0.0\ninfo: {title: x, version: \"1\"}\npaths: {}\ntype: {\n",
	} {
		definition := Parse("openapi", "x.yaml", content, Options{}, DefaultLimits(), &fixtureResolver{})
		assertError(t, definition.Validation, "OpenAPI paths are missing")
	}
}

func TestJSONSchemaMessageSelectsDefsAndReportsMissing(t *testing.T) {
	content := `{"$defs":{"User":{"type":"object","properties":{"id":{"type":"integer"}}}}}`
	selected := Parse("json-schema", "x.json", content, Options{Message: "User"}, DefaultLimits(), &fixtureResolver{})
	if selected.Messages[0]["name"] != "User" {
		t.Fatalf("message: %#v", selected.Messages[0])
	}
	if got := field(t, selected.Messages[0], 0); got["name"] != "id" {
		t.Fatalf("field: %#v", got)
	}
	missing := Parse("json-schema", "x.json", content, Options{Message: "Nope"}, DefaultLimits(), &fixtureResolver{})
	assertError(t, missing.Validation, "message not found: Nope")
}

func TestImportsAreConfinedAndCycleSafe(t *testing.T) {
	resolver := &fixtureResolver{}
	traversals := []string{"../../../../etc/passwd", "/etc/passwd", `C:\secret.json`, "bad\x00.json"}
	for _, ref := range traversals {
		body := `{"$ref":` + quoteJSON(ref) + `#/X}`
		definition := Parse("json-schema", "json-schema/user.json", body, Options{}, DefaultLimits(), resolver)
		if definition.Validation.OK {
			t.Fatalf("traversal %q accepted", ref)
		}
	}
	if len(resolver.reads) != 0 {
		t.Fatalf("resolver read escaped paths: %#v", resolver.reads)
	}

	cycle := Parse("graphql", "graphql/schema.graphql",
		`#import "schema.graphql"`+"\ntype Query { user: String }\n", Options{}, DefaultLimits(), resolver)
	if !cycle.Validation.OK || len(cycle.Validation.Warnings) != 1 ||
		cycle.Validation.Warnings[0].Message != "import cycle skipped" {
		t.Fatalf("cycle: %#v", cycle.Validation)
	}
}

func TestImportCaps(t *testing.T) {
	resolver := mapResolver{
		"b.proto": `syntax = "proto3"; message B { string b = 1; }`,
		"c.proto": `syntax = "proto3"; message C { string c = 1; }`,
	}
	byImports := Parse("proto", "a.proto",
		"import \"b.proto\";\nimport \"c.proto\";\nmessage A { string a = 1; }\nservice S { rpc Op (A) returns (A); }",
		Options{}, Limits{MaxImports: 1}, resolver)
	assertError(t, byImports.Validation, "max_imports exceeded")

	nested := mapResolver{
		"b.proto": "import \"c.proto\";\nmessage B { string b = 1; }\n",
		"c.proto": "message C { string c = 1; }\n",
	}
	byDepth := Parse("proto", "a.proto", "import \"b.proto\";\nmessage A { string a = 1; }\nservice S { rpc Op (A) returns (A); }",
		Options{}, Limits{MaxDepth: 1}, nested)
	assertError(t, byDepth.Validation, "max_depth exceeded")
}

func TestOperationOptionOverridesAdapterResult(t *testing.T) {
	definition := parseFixture(t, "openapi/api.yaml", "openapi", withOperation("nope"))
	assertError(t, definition.Validation, "operation not found: nope; candidates: getUser, createUser")
	if definition.Operation != "nope" {
		t.Fatalf("operation: %q", definition.Operation)
	}
}

func diagnosticMessages(errors []Diagnostic) []string {
	messages := make([]string, len(errors))
	for i, error := range errors {
		messages[i] = error.Message
	}
	return messages
}

func quoteJSON(value string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\x00", `\u0000`)
	return `"` + replacer.Replace(value) + `"`
}

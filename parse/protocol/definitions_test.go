package protocol

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/erikwang2013/apidoc-go/model"
)

func TestDefinitionsEventFallback(t *testing.T) {
	action := &model.Action{
		Method:   "GET",
		Protocol: "websocket",
		Events: []model.DocEvent{
			{Direction: "in", Name: "tick", Type: "object", Desc: "server tick", Mock: `{"t":1}`},
			{Direction: "out", Name: "ping", Type: "string"},
		},
	}
	definitions, validation := Definitions(action, nil)
	if !validation.OK || len(validation.Errors) != 0 || len(validation.Warnings) != 0 {
		t.Fatalf("validation: %#v", validation)
	}
	if len(definitions) != 1 {
		t.Fatalf("definitions: %#v", definitions)
	}
	definition := definitions[0]
	if definition.Format != "asyncapi" || definition.Source != "inline" || definition.Operation != "GET" {
		t.Fatalf("definition: %#v", definition)
	}
	if definition.Meta["protocol"] != "websocket" || len(definition.Messages) != 2 {
		t.Fatalf("definition: %#v", definition)
	}
	if definition.Messages[0]["direction"] != "response" || definition.Messages[1]["direction"] != "request" {
		t.Fatalf("directions: %#v", definition.Messages)
	}
	if definition.Messages[0]["sample"] != `{"t":1}` {
		t.Fatalf("sample: %#v", definition.Messages[0])
	}
	encoded, err := json.Marshal(definition)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), `"errors":[]`) {
		t.Fatalf("empty diagnostics must serialize as arrays: %s", encoded)
	}
}

func TestDefinitionsRendersDeclaredFile(t *testing.T) {
	action := &model.Action{
		Protocol: "webhook",
		Definitions: []model.Definition{{
			File:    "json-schema/user.json",
			Sample:  `{"id":1,"name":"a"}`,
			Format:  "json-schema",
			Message: "",
		}},
	}
	definitions, validation := Definitions(action, &fixtureResolver{})
	if !validation.OK || len(definitions) != 1 {
		t.Fatalf("validation: %#v definitions: %#v", validation, definitions)
	}
	definition := definitions[0]
	if definition.Source != "json-schema/user.json" || definition.Format != "json-schema" {
		t.Fatalf("definition: %#v", definition)
	}
	if definition.Messages[0]["name"] != "User" {
		t.Fatalf("message: %#v", definition.Messages[0])
	}
	if sample := definition.Messages[0]["sampleValidation"].(SampleValidation); !sample.OK {
		t.Fatalf("sample: %#v", sample)
	}
}

func TestDefinitionsReadsSampleFile(t *testing.T) {
	action := &model.Action{
		Protocol: "webhook",
		Definitions: []model.Definition{{
			File:       "json-schema/user.json",
			SampleFile: "json-schema/user-sample.json",
		}},
	}
	definitions, validation := Definitions(action, &fixtureResolver{})
	if !validation.OK || len(definitions) != 1 {
		t.Fatalf("validation: %#v", validation)
	}
	if sample := definitions[0].Messages[0]["sampleValidation"].(SampleValidation); !sample.OK {
		t.Fatalf("sample: %#v", sample)
	}
}

func TestDefinitionsMissingFile(t *testing.T) {
	action := &model.Action{
		Protocol:    "webhook",
		Definitions: []model.Definition{{File: "missing.json"}},
	}
	definitions, validation := Definitions(action, &fixtureResolver{})
	if validation.OK || len(definitions) != 1 {
		t.Fatalf("validation: %#v", validation)
	}
	error := validation.Errors[0]
	if error.Message != "definition file not found or outside allowed roots" || error.Severity != "error" {
		t.Fatalf("error: %#v", error)
	}
	if definitions[0].Format != "json-schema" || definitions[0].Source != "missing.json" {
		t.Fatalf("definition: %#v", definitions[0])
	}
}

func TestDefinitionsInlineContent(t *testing.T) {
	action := &model.Action{
		Definitions: []model.Definition{{
			Inline: `{"type":"object","properties":{"id":{"type":"integer"}}}`,
			Format: "json-schema",
		}},
	}
	definitions, validation := Definitions(action, nil)
	if !validation.OK || len(definitions) != 1 {
		t.Fatalf("validation: %#v", validation)
	}
	if definitions[0].Source != "inline" || definitions[0].Messages[0]["name"] != "Body" {
		t.Fatalf("definition: %#v", definitions[0])
	}
}

func TestDefinitionsInlineAnnotationFallback(t *testing.T) {
	action := &model.Action{
		Definitions: []model.Definition{{}},
		Params:      []model.Param{{Name: "id", Type: "int", Required: true, Desc: "user id"}},
		Responses:   []model.Response{{Name: "ok", Type: "object", Fields: []model.Param{{Name: "name", Type: "string"}}}},
	}
	definitions, validation := Definitions(action, nil)
	if !validation.OK || len(definitions) != 1 {
		t.Fatalf("validation: %#v", validation)
	}
	definition := definitions[0]
	if definition.Format != "inline" || definition.Source != "inline" || len(definition.Messages) != 2 {
		t.Fatalf("definition: %#v", definition)
	}
	if got := field(t, definition.Messages[0], 0); got["name"] != "id" || got["require"] != true {
		t.Fatalf("request field: %#v", got)
	}
	if got := field(t, definition.Messages[1], 0); got["name"] != "name" {
		t.Fatalf("response field: %#v", got)
	}
}

func TestDefinitionsNoDeclarationsNoEvents(t *testing.T) {
	definitions, validation := Definitions(&model.Action{}, nil)
	if !validation.OK || len(definitions) != 0 {
		t.Fatalf("definitions: %#v validation: %#v", definitions, validation)
	}
	if definitions == nil {
		t.Fatal("definitions must be an empty array, not nil")
	}
}

func TestActionWithoutDefinitionsSerializesUnchanged(t *testing.T) {
	encoded, err := json.Marshal(model.Action{})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), `"definitions"`) {
		t.Fatalf("definition inputs must be omitted when unused: %s", encoded)
	}
}

func TestDefinitionsAutoFormat(t *testing.T) {
	action := &model.Action{
		Protocol:    "webhook",
		Definitions: []model.Definition{{File: "json-schema/user.json", Format: "auto"}},
	}
	definitions, validation := Definitions(action, &fixtureResolver{})
	if !validation.OK || definitions[0].Format != "json-schema" {
		t.Fatalf("validation: %#v definition: %#v", validation, definitions)
	}
}

func TestDefinitionsAutoFormatOnFailureUsesCatalogDefault(t *testing.T) {
	action := &model.Action{
		Protocol:    "webhook",
		Definitions: []model.Definition{{File: "missing.json", Format: "auto", Operation: "declared"}},
	}
	definitions, validation := Definitions(action, &fixtureResolver{})
	if validation.OK {
		t.Fatalf("validation: %#v", validation)
	}
	definition := definitions[0]
	if definition.Format != "json-schema" || definition.Operation != "" || definition.Source != "missing.json" {
		t.Fatalf("definition: %#v", definition)
	}
	if validation.Errors[0].Message != "definition file not found or outside allowed roots" {
		t.Fatalf("error: %#v", validation.Errors[0])
	}

	unknown := &model.Action{
		Protocol:    "custom",
		Definitions: []model.Definition{{File: "missing.txt", Format: "auto"}},
	}
	definitions, _ = Definitions(unknown, &fixtureResolver{})
	if definitions[0].Format != "auto" {
		t.Fatalf("unknown protocol must keep the literal auto format: %#v", definitions[0])
	}
}

func TestDefinitionsHTTPAndRESTDefaultToOpenAPI(t *testing.T) {
	for _, protocol := range []string{"http", "rest"} {
		action := &model.Action{
			Protocol:    protocol,
			Definitions: []model.Definition{{Inline: `{"type":"object"}`}},
		}
		definitions, _ := Definitions(action, nil)
		if definitions[0].Format != "openapi" {
			t.Fatalf("%s default format: %#v", protocol, definitions[0])
		}
	}
}

func TestDefinitionsFilelessDeclarations(t *testing.T) {
	inline := &model.Action{
		Protocol:    "soap",
		Params:      []model.Param{{Name: "id", Type: "int"}},
		Definitions: []model.Definition{{Format: "inline"}},
	}
	definitions, validation := Definitions(inline, nil)
	if !validation.OK || definitions[0].Format != "inline" || definitions[0].Source != "inline" || len(definitions[0].Messages) != 1 {
		t.Fatalf("inline: %#v validation: %#v", definitions[0], validation)
	}

	noDefault := &model.Action{Protocol: "custom", Definitions: []model.Definition{{}}}
	definitions, _ = Definitions(noDefault, nil)
	if definitions[0].Format != "inline" {
		t.Fatalf("protocol without default format: %#v", definitions[0])
	}

	for _, declaration := range []model.Definition{{}, {Format: "wsdl"}} {
		action := &model.Action{Protocol: "webhook", Definitions: []model.Definition{declaration}}
		definitions, validation = Definitions(action, nil)
		definition := definitions[0]
		if !validation.OK || definition.Format != "pending" || definition.Source != "" || definition.Operation != "" ||
			len(definition.Messages) != 0 || definition.Meta == nil || len(validation.Warnings) != 1 {
			t.Fatalf("pending %#v: definition %#v validation %#v", declaration, definition, validation)
		}
		if validation.Warnings[0].Message != "definition file required for webhook" {
			t.Fatalf("warning: %#v", validation.Warnings[0])
		}
	}
}

func TestDefinitionsRejectTraversalBeforeResolver(t *testing.T) {
	for _, file := range []string{"../secret.json", "/etc/passwd", `dir\..\secret.json`} {
		action := &model.Action{Protocol: "webhook", Definitions: []model.Definition{{File: file}}}
		resolver := &fixtureResolver{}
		definitions, validation := Definitions(action, resolver)
		if validation.OK || definitions[0].Format != "json-schema" {
			t.Fatalf("%q: %#v %#v", file, definitions[0], validation)
		}
		if len(resolver.reads) != 0 {
			t.Fatalf("%q: resolver must not see escaped paths: %#v", file, resolver.reads)
		}
	}

	action := &model.Action{Protocol: "webhook", Definitions: []model.Definition{{
		File: "json-schema/user.json", SampleFile: "../sample.json",
	}}}
	resolver := &fixtureResolver{}
	definitions, validation := Definitions(action, resolver)
	if validation.OK || validation.Errors[0].Message != "sample file not found or outside allowed roots" {
		t.Fatalf("sample traversal: %#v", validation)
	}
	if len(resolver.reads) != 1 || resolver.reads[0] != "json-schema/user.json" {
		t.Fatalf("resolver reads: %#v", resolver.reads)
	}
	if definitions[0].Format != "json-schema" {
		t.Fatalf("definition: %#v", definitions[0])
	}
}

func TestDefinitionsEnforceFileLimits(t *testing.T) {
	limits := DefaultLimits()
	big := strings.Repeat("x", limits.MaxFileBytes+1)
	lotsOfLines := strings.Repeat("x\n", limits.MaxLines+1)
	cases := []struct {
		name, content, message string
		sample                 bool
	}{
		{"definition bytes", big, "file exceeds max_file_bytes", false},
		{"definition lines", lotsOfLines, "file exceeds max_lines", false},
		{"sample bytes", big, "file exceeds max_file_bytes", true},
		{"sample lines", lotsOfLines, "file exceeds max_lines", true},
	}
	for _, c := range cases {
		resolver := mapResolver{
			"def.json":    `{"type":"object"}`,
			"sample.json": c.content,
		}
		declaration := model.Definition{File: "def.json", Format: "json-schema", Operation: "declared"}
		if c.sample {
			declaration.SampleFile = "sample.json"
		} else {
			resolver["def.json"] = c.content
		}
		definitions, validation := Definitions(&model.Action{Definitions: []model.Definition{declaration}}, resolver)
		if validation.OK || validation.Errors[0].Message != c.message {
			t.Fatalf("%s: validation %#v", c.name, validation)
		}
		if c.sample && definitions[0].Messages[0]["sample"] != "" {
			t.Fatalf("%s: oversized sample must not be retained: %#v", c.name, definitions[0].Messages[0])
		}
		// PHP keeps the declared operation when the file resolved but read failed.
		if definitions[0].Operation != "declared" {
			t.Fatalf("%s: operation = %q", c.name, definitions[0].Operation)
		}
	}
}

func TestDefinitionsNoDeclarationsStayEmptyEvenWithAnnotations(t *testing.T) {
	// Intentional divergence from PHP (which renders annotation params inline):
	// Phase 4 preserves the pre-existing /ui/v1/detail shape for docs with no
	// @definition annotation; events fall back below, params do not.
	action := &model.Action{
		Params:    []model.Param{{Name: "id", Type: "int"}},
		Responses: []model.Response{{Name: "ok", Type: "object"}},
	}
	definitions, validation := Definitions(action, nil)
	if !validation.OK || len(definitions) != 0 {
		t.Fatalf("definitions: %#v validation: %#v", definitions, validation)
	}
}

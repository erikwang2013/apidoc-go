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

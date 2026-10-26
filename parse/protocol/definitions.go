package protocol

import (
	"github.com/erikwang2013/apidoc-go/model"
)

// Definitions converts an action's declared definitions into UIC-1 shapes and
// aggregates their diagnostics. With no declarations it falls back to the
// action's event messages (asyncapi inline), matching the legacy detail output.
// File declarations are read through resolver; the parser itself never touches
// the filesystem.
func Definitions(action *model.Action, resolver Resolver) ([]Definition, Validation) {
	validation := Validation{OK: true, Errors: []Diagnostic{}, Warnings: []Diagnostic{}}
	if action == nil {
		return []Definition{}, validation
	}
	if len(action.Definitions) == 0 {
		return eventDefinitions(action), validation
	}
	if resolver == nil {
		resolver = NoResolver{}
	}
	rendered := make([]Definition, 0, len(action.Definitions))
	for _, declaration := range action.Definitions {
		definition := renderDefinition(action, declaration, resolver)
		rendered = append(rendered, definition)
		validation.Errors = append(validation.Errors, definition.Validation.Errors...)
		validation.Warnings = append(validation.Warnings, definition.Validation.Warnings...)
	}
	validation.OK = len(validation.Errors) == 0
	return rendered, validation
}

func renderDefinition(action *model.Action, declaration model.Definition, resolver Resolver) Definition {
	if declaration.File == "" && declaration.Inline == "" {
		return inlineDefinition(action)
	}
	source := declaration.File
	if source == "" {
		source = "inline"
	}
	content := declaration.Inline
	if content == "" {
		_, read, ok := resolver.Read(declaration.File)
		if !ok {
			message := resolver.Failure()
			if message == "" {
				message = "definition file not found or outside allowed roots"
			}
			return failedDefinition(definitionFormat(action, declaration, ""), source, declaration.Operation, message)
		}
		content = read
	}
	format := definitionFormat(action, declaration, content)
	options := Options{
		Operation:      declaration.Operation,
		Message:        declaration.Message,
		Direction:      declaration.Direction,
		Sample:         declaration.Sample,
		SampleEncoding: declaration.SampleEncoding,
		Lenient:        declaration.Strict != nil && !*declaration.Strict,
	}
	var sampleErrors []Diagnostic
	if declaration.SampleFile != "" {
		_, sample, ok := resolver.Read(declaration.SampleFile)
		if !ok {
			message := resolver.Failure()
			if message == "" {
				message = "sample file not found or outside allowed roots"
			}
			sampleErrors = append(sampleErrors, Diagnostic{Severity: "error", Message: message})
		} else {
			options.Sample = sample
		}
	}
	definition := Parse(format, source, content, options, DefaultLimits(), resolver)
	if len(sampleErrors) > 0 {
		definition.Validation.Errors = append(sampleErrors, definition.Validation.Errors...)
		definition.Validation.OK = false
	}
	return definition
}

func definitionFormat(action *model.Action, declaration model.Definition, content string) string {
	if declaration.Format != "" {
		return declaration.Format
	}
	format := Detect(declaration.File, content)
	if format == "" {
		format = defaultDefinitionFormat(action.Protocol)
	}
	return format
}

func defaultDefinitionFormat(protocol string) string {
	switch protocol {
	case "soap":
		return "wsdl"
	case "graphql":
		return "graphql"
	case "grpc", "dubbo":
		return "proto"
	case "thrift":
		return "thrift"
	case "hessian":
		return "hessian"
	case "jsonrpc":
		return "json-rpc"
	case "xmlrpc":
		return "xml-rpc"
	case "webhook", "webhooks", "mcp", "a2a":
		return "json-schema"
	case "odata":
		return "edmx"
	}
	return ""
}

func failedDefinition(format, source, operation, message string) Definition {
	return Definition{
		Format:    format,
		Source:    source,
		Operation: operation,
		Meta:      map[string]any{},
		Messages:  []map[string]any{},
		Validation: Validation{
			OK:       false,
			Errors:   []Diagnostic{{Severity: "error", Message: message}},
			Warnings: []Diagnostic{},
		},
	}
}

func inlineDefinition(action *model.Action) Definition {
	messages := []map[string]any{}
	if fields := inlineParamFields(action.Params); len(fields) > 0 {
		messages = append(messages, newMessage("Request", "request", fields))
	}
	if fields := inlineResponseFields(action.Responses); len(fields) > 0 {
		messages = append(messages, newMessage("Response", "response", fields))
	}
	return Definition{
		Format:    "inline",
		Source:    "inline",
		Operation: "",
		Meta:      map[string]any{},
		Messages:  messages,
		Validation: Validation{
			OK: true, Errors: []Diagnostic{}, Warnings: []Diagnostic{},
		},
	}
}

func inlineParamFields(params []model.Param) []map[string]any {
	fields := make([]map[string]any, 0, len(params))
	for _, param := range params {
		fields = append(fields, map[string]any{
			"name": param.Name, "type": param.Type, "require": param.Required, "desc": param.Desc,
		})
	}
	return fields
}

func inlineResponseFields(responses []model.Response) []map[string]any {
	fields := []map[string]any{}
	for _, response := range responses {
		if len(response.Fields) > 0 {
			for _, field := range response.Fields {
				fields = append(fields, map[string]any{
					"name": field.Name, "type": field.Type, "require": field.Required, "desc": field.Desc,
				})
			}
			continue
		}
		if response.Name != "" {
			fields = append(fields, map[string]any{
				"name": response.Name, "type": response.Type, "require": false, "desc": response.Desc,
			})
		}
	}
	return fields
}

func eventDefinitions(action *model.Action) []Definition {
	if len(action.Events) == 0 {
		return []Definition{}
	}
	messages := make([]map[string]any, 0, len(action.Events))
	for _, event := range action.Events {
		direction := "request"
		if event.Direction == "in" {
			direction = "response"
		}
		message := map[string]any{
			"name":      event.Name,
			"direction": direction,
			"fields": []map[string]any{{
				"name": "body", "type": event.Type, "desc": event.Desc, "require": true,
			}},
			"sampleValidation": okSampleValidation(),
		}
		if event.Mock != "" {
			message["sample"] = event.Mock
		}
		messages = append(messages, message)
	}
	return []Definition{{
		Format:    "asyncapi",
		Source:    "inline",
		Operation: action.Method,
		Meta:      map[string]any{"protocol": action.Protocol},
		Messages:  messages,
		Validation: Validation{
			OK: true, Errors: []Diagnostic{}, Warnings: []Diagnostic{},
		},
	}}
}

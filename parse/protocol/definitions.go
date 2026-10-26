package protocol

import (
	"strings"

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
	source := declaration.File
	if source == "" {
		source = "inline"
	}
	content := declaration.Inline
	if declaration.File == "" && content == "" {
		// PHP: an inline format (or a protocol without a default format) renders
		// the annotation params/responses; anything else waits for a file.
		format := declaration.Format
		if format == "" {
			format = "auto"
		}
		if format == "inline" || defaultDefinitionFormat(action.Protocol) == "" {
			return inlineDefinition(action)
		}
		return pendingDefinition(action.Protocol)
	}
	if declaration.File != "" && content == "" {
		normalized, valid := normalizeDefinitionPath(declaration.File)
		var message string
		var ok bool
		if valid {
			content, message, ok = readDefinitionFile(resolver, normalized)
		}
		if !valid || !ok {
			if message == "" {
				message = "definition file not found or outside allowed roots"
			}
			if isReadFailure(message) {
				return failedDefinition(definitionFormat(action, declaration, ""), source, declaration.Operation, message)
			}
			return failedDefinition(resolveFailureFormat(action, declaration), source, "", message)
		}
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
		if normalized, valid := normalizeDefinitionPath(declaration.SampleFile); valid {
			sample, message, ok := readDefinitionFile(resolver, normalized)
			if ok {
				options.Sample = sample
			} else {
				if message == "" {
					message = "sample file not found or outside allowed roots"
				}
				sampleErrors = append(sampleErrors, Diagnostic{Severity: "error", Message: message})
			}
		} else {
			sampleErrors = append(sampleErrors, Diagnostic{
				Severity: "error", Message: "sample file not found or outside allowed roots",
			})
		}
	}
	definition := Parse(format, source, content, options, DefaultLimits(), resolver)
	if len(sampleErrors) > 0 {
		definition.Validation.Errors = append(sampleErrors, definition.Validation.Errors...)
		definition.Validation.OK = false
	}
	return definition
}

// readDefinitionFile applies ProtocolContext's limits to resolver content; the
// resolver only returns bytes, so stat/read caps are enforced here.
var readFailureMessages = map[string]bool{
	"unable to stat file":         true,
	"file exceeds max_file_bytes": true,
	"unable to read file":         true,
	"file exceeds max_lines":      true,
}

func isReadFailure(message string) bool { return readFailureMessages[message] }

func readDefinitionFile(resolver Resolver, path string) (string, string, bool) {
	if resolver == nil {
		resolver = NoResolver{}
	}
	_, content, ok := resolver.Read(path)
	if !ok {
		return "", resolver.Failure(), false
	}
	limits := DefaultLimits()
	if len(content) > limits.MaxFileBytes {
		return "", "file exceeds max_file_bytes", false
	}
	if strings.Count(content, "\n")+1 > limits.MaxLines {
		return "", "file exceeds max_lines", false
	}
	return content, "", true
}

// normalizeDefinitionPath mirrors ProtocolPaths::resolve: definitions are
// root-relative, so absolute paths and any ".." segment are rejected before the
// resolver (which owns root/symlink confinement) sees them.
func normalizeDefinitionPath(path string) (string, bool) {
	if path == "" || strings.ContainsRune(path, '\x00') {
		return "", false
	}
	if strings.HasPrefix(path, "/") {
		return "", false
	}
	if len(path) >= 3 && isASCIIAlpha(path[0]) && path[1] == ':' && (path[2] == '\\' || path[2] == '/') {
		return "", false
	}
	slashed := strings.ReplaceAll(path, "\\", "/")
	for _, segment := range strings.Split(slashed, "/") {
		if segment == ".." {
			return "", false
		}
	}
	return normalizePath(slashed)
}

func definitionFormat(action *model.Action, declaration model.Definition, content string) string {
	format := declaration.Format
	if format == "" || format == "auto" {
		format = Detect(declaration.File, content)
		if format == "" {
			format = defaultDefinitionFormat(action.Protocol)
		}
	}
	return format
}

// resolveFailureFormat mirrors PHP's not-found branch: auto falls back to the
// catalog default, keeping the literal "auto" when there is none.
func resolveFailureFormat(action *model.Action, declaration model.Definition) string {
	if declaration.Format != "" && declaration.Format != "auto" {
		return declaration.Format
	}
	if format := defaultDefinitionFormat(action.Protocol); format != "" {
		return format
	}
	return "auto"
}

func defaultDefinitionFormat(protocol string) string {
	switch protocol {
	case "http", "rest":
		return "openapi"
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
	case "jsonrpc", "json-rpc":
		return "json-rpc"
	case "xmlrpc", "xml-rpc":
		return "xml-rpc"
	case "webhook", "webhooks", "mcp", "a2a":
		return "json-schema"
	case "odata":
		return "edmx"
	}
	return ""
}

func pendingDefinition(protocol string) Definition {
	return Definition{
		Format:    "pending",
		Source:    "",
		Operation: "",
		Meta:      map[string]any{},
		Messages:  []map[string]any{},
		Validation: Validation{
			OK:     true,
			Errors: []Diagnostic{},
			Warnings: []Diagnostic{{
				Severity: "warning", Message: "definition file required for " + protocol,
			}},
		},
	}
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

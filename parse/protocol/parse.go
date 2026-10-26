package protocol

import (
	"strings"
)

// Formats are the definition formats implemented by this package.
var Formats = []string{
	"openapi", "asyncapi", "graphql", "proto", "wsdl",
	"edmx", "json-schema", "json-rpc", "xml-rpc",
}

// Definition is the UIC-1 normalized definition.
type Definition struct {
	Format     string           `json:"format"`
	Source     string           `json:"source"`
	Operation  string           `json:"operation"`
	Meta       map[string]any   `json:"meta"`
	Messages   []map[string]any `json:"messages"`
	Validation Validation       `json:"validation"`
}

// Detect infers a definition format from the declared path and, for structured
// formats, the content. It mirrors ProtocolRegistry::detect.
func Detect(relativePath, content string) string {
	basename := relativePath
	if index := strings.LastIndexByte(basename, '/'); index >= 0 {
		basename = basename[index+1:]
	}
	extension := ""
	if index := strings.LastIndexByte(basename, '.'); index >= 0 {
		extension = strings.ToLower(basename[index+1:])
	}
	switch extension {
	case "proto":
		return "proto"
	case "thrift":
		return "thrift"
	case "graphql", "gql":
		return "graphql"
	case "wsdl":
		return "wsdl"
	case "json", "yaml", "yml":
		return detectStructured(content)
	case "xml":
		return detectXML(content)
	}
	return ""
}

func detectStructured(content string) string {
	for _, rule := range [][2]string{
		{"openapi", "openapi"},
		{"swagger", "openapi"},
		{"asyncapi", "asyncapi"},
		{"methods", "json-rpc"},
	} {
		if hasKeyColon(content, rule[0]) {
			return rule[1]
		}
	}
	for _, key := range []string{"$schema", "$ref", "type", "properties"} {
		if hasKeyColon(content, key) {
			return "json-schema"
		}
	}
	return ""
}

// hasKeyColon matches ["']?key["']?\s*: case-insensitively.
func hasKeyColon(content, key string) bool {
	lowered := strings.ToLower(content)
	key = strings.ToLower(key)
	offset := 0
	for {
		index := strings.Index(lowered[offset:], key)
		if index < 0 {
			return false
		}
		after := offset + index + len(key)
		rest := strings.TrimLeft(content[after:], " \t\n\r\f\v")
		rest = strings.TrimPrefix(rest, "\"")
		rest = strings.TrimPrefix(rest, "'")
		rest = strings.TrimLeft(rest, " \t\n\r\f\v")
		if strings.HasPrefix(rest, ":") {
			return true
		}
		offset = after
	}
}

func detectXML(content string) string {
	lowered := strings.ToLower(content)
	switch {
	case strings.Contains(lowered, "<edmx:"):
		return "edmx"
	case strings.Contains(lowered, "<definitions") && strings.Contains(lowered, "wsdl"):
		return "wsdl"
	case strings.Contains(lowered, "<methodcall") || strings.Contains(lowered, "<methodresponse"):
		return "xml-rpc"
	case strings.Contains(lowered, "<envelope") || strings.Contains(lowered, "<definitions"):
		return "wsdl"
	}
	return ""
}

// Parse parses content as format. Resolver supplies imports and $ref targets;
// this package never reads the filesystem or network itself.
func Parse(format, source, content string, options Options, limits Limits, resolver Resolver) Definition {
	ctx := NewContext(source, content, options, limits, resolver)
	operation := ""
	meta := map[string]any{}
	messages := []map[string]any{}
	if read, ok := ctx.Read(); ok {
		var parsed Parsed
		var err *ParseError
		switch format {
		case "openapi":
			parsed, err = parseOpenAPI(ctx, read)
		case "asyncapi":
			parsed, err = parseAsyncAPI(ctx, read)
		case "graphql":
			parsed, err = parseGraphQL(ctx, read)
		case "proto":
			parsed, err = parseProto(ctx, read)
		case "wsdl":
			parsed, err = parseWSDL(ctx, read)
		case "edmx":
			parsed, err = parseEDMX(ctx, read)
		case "json-schema":
			parsed, err = parseJSONSchema(ctx, read)
		case "json-rpc":
			parsed, err = parseJSONRPC(ctx, read)
		case "xml-rpc":
			parsed, err = parseXMLRPC(ctx, read)
		default:
			ctx.Unsupported("unsupported definition format: "+format, 0, 0)
		}
		if err != nil {
			ctx.Error(err.Message, err.Line, err.Column)
		} else if parsed.Messages != nil {
			operation, meta, messages = parsed.Operation, parsed.Meta, parsed.Messages
		}
		if ctx.Options().Operation != "" {
			operation = ctx.Options().Operation
		}
	}
	errors := ctx.Errors()
	warnings := ctx.Warnings()
	return Definition{
		Format:    format,
		Source:    source,
		Operation: operation,
		Meta:      meta,
		Messages:  messages,
		Validation: Validation{
			OK:       len(errors) == 0,
			Errors:   errors,
			Warnings: warnings,
		},
	}
}

package protocol

import (
	"strings"
)

func parseJSONRPC(ctx *Context, content string) (Parsed, *ParseError) {
	document, message := ParseJSON(content)
	if message != "" {
		return emptyParsed(), &ParseError{Message: "JSON-RPC JSON parse error: " + message}
	}
	if !document.IsArrayLike() {
		return emptyParsed(), &ParseError{Message: "JSON-RPC JSON parse error: No error"}
	}
	methods := document.Get("methods")
	if methods == nil || !methods.IsArrayLike() {
		methods = Object()
	}
	names := methods.Keys()
	if len(names) == 0 {
		ctx.Error("JSON-RPC methods are missing", 0, 0)
		return emptyParsed(), nil
	}
	wanted := ctx.Options().Operation
	if wanted == "" {
		if len(names) == 1 {
			wanted = names[0]
		} else {
			ctx.Error("operation required; candidates: "+strings.Join(names, ", "), 0, 0)
			return emptyParsed(), nil
		}
	}
	method := methods.Get(wanted)
	if method == nil || !method.IsArrayLike() {
		ctx.Error("operation not found: "+wanted+"; candidates: "+strings.Join(names, ", "), 0, 0)
		return emptyParsed(), nil
	}
	paramsSchema := method.Get("params")
	if paramsSchema == nil || !paramsSchema.IsArrayLike() {
		paramsSchema = Object()
	}
	resultSchema := method.Get("result")
	if resultSchema == nil || !resultSchema.IsArrayLike() {
		resultSchema = Object()
	}
	paramsSchema = expandSchema(paramsSchema, document, ctx, LoaderJSON, "")
	resultSchema = expandSchema(resultSchema, document, ctx, LoaderJSON, "")
	refs := refsFor(document, LoaderJSON)
	messages := []map[string]any{
		newMessage("Params", "request", SchemaFields(paramsSchema, ctx, refs, 0)),
		newMessage("Result", "response", SchemaFields(resultSchema, ctx, refs, 0)),
	}
	sample := ctx.Options().Sample
	if sample != "" {
		validateJSONRPCSample(sample, wanted, paramsSchema, resultSchema, messages, refs, ctx)
	}
	return Parsed{Operation: wanted, Messages: messages, Meta: map[string]any{}}, nil
}

func validateJSONRPCSample(sample, method string, paramsSchema, resultSchema *Node, messages []map[string]any, refs *RefCtx, ctx *Context) {
	data, message := ParseJSON(sample)
	if message == "" && !data.IsArrayLike() {
		message = "No error"
	}
	if message != "" {
		messages[0]["sampleValidation"] = SampleValidation{
			OK: false,
			Errors: []SampleError{{
				Path: "$", Severity: "error", Message: "JSON sample parse error: " + message,
			}},
			Warnings: []SampleError{},
		}
		return
	}
	errors := []SampleError{}
	jsonrpc := StringNode("")
	if node := data.Get("jsonrpc"); node != nil && node.Kind != nullNode {
		jsonrpc = node
	}
	if !jsonrpc.StrictEq(StringNode("2.0")) {
		errors = append(errors, SampleError{Path: "$.jsonrpc", Severity: "error", Message: "jsonrpc must be 2.0"})
	}
	if actual := data.Get("method"); actual != nil && actual.Kind != nullNode {
		if !actual.StrictEq(StringNode(method)) {
			errors = append(errors, SampleError{Path: "$.method", Severity: "error", Message: "method mismatch"})
		}
	}
	if data.Has("params") {
		for _, failure := range ValidateSchema(paramsSchema, data.Get("params"), refs, ctx, "$", 0) {
			errors = append(errors, SampleError{
				Path: nestedSamplePath(failure.Path), Severity: "error", Message: failure.Message,
			})
		}
		messages[0]["sampleValidation"] = SampleValidation{
			OK: len(errors) == 0, Errors: errors, Warnings: []SampleError{},
		}
	}
	if data.Has("result") {
		resultErrors := []SampleError{}
		for _, failure := range ValidateSchema(resultSchema, data.Get("result"), refs, ctx, "$", 0) {
			resultErrors = append(resultErrors, SampleError{
				Path: nestedSamplePath(failure.Path), Severity: "error", Message: failure.Message,
			})
		}
		messages[1]["sampleValidation"] = SampleValidation{
			OK: len(resultErrors) == 0, Errors: resultErrors, Warnings: []SampleError{},
		}
	}
}

// nestedSamplePath is '$' . substr($path, 1).
func nestedSamplePath(path string) string {
	if len(path) <= 1 {
		return "$"
	}
	return "$" + path[1:]
}

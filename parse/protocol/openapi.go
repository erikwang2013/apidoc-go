package protocol

import (
	"strings"
)

var openAPIMethods = []string{"get", "post", "put", "patch", "delete", "options", "head", "trace"}
var openAPIContentTypes = []string{"application/json", "*/*", "application/*"}

type openAPIOperation struct {
	path        string
	method      string
	operationID string
	operation   *Node
	pathItem    *Node
	root        *Node
	file        string
}

func (o openAPIOperation) displayName() string {
	if o.operationID == "" {
		return o.signature()
	}
	return o.operationID
}

func (o openAPIOperation) signature() string {
	return strings.ToUpper(o.method) + " " + o.path
}

func parseOpenAPI(ctx *Context, content string) (Parsed, *ParseError) {
	document, err := decodeOpenAPI(content)
	if err != nil {
		return emptyParsed(), err
	}
	version := document.Get("openapi").VersionString()
	swagger := document.Get("swagger").VersionString()
	if version == "" && swagger == "" {
		ctx.Error("OpenAPI version is missing", 0, 0)
	} else if version != "" && !strings.HasPrefix(version, "3.") {
		ctx.Unsupported("unsupported OpenAPI version: "+version, 0, 0)
	} else if swagger != "" && !strings.HasPrefix(swagger, "2.") {
		ctx.Unsupported("unsupported Swagger version: "+swagger, 0, 0)
	}
	paths := document.Get("paths")
	if paths == nil || !paths.IsArrayLike() || paths.IsEmpty() {
		ctx.Error("OpenAPI paths are missing", 0, 0)
		return emptyParsed(), nil
	}
	operations := collectOpenAPIOperations(paths, document, ctx)
	if len(operations) == 0 {
		ctx.Error("OpenAPI operations are missing", 0, 0)
		return emptyParsed(), nil
	}
	selected := selectOpenAPIOperation(operations, ctx.Options().Operation, ctx)
	refs := refsFor(document, LoaderJSONOrYAML)

	var requestFields []map[string]any
	var parameters []*Node
	if list := selected.pathItem.Get("parameters"); list != nil && list.IsArrayLike() {
		for _, entry := range entries(list) {
			parameters = append(parameters, entry.Value)
		}
	}
	if list := selected.operation.Get("parameters"); list != nil && list.IsArrayLike() {
		for _, entry := range entries(list) {
			parameters = append(parameters, entry.Value)
		}
	}
	for _, parameter := range parameters {
		if !parameter.IsArrayLike() {
			continue
		}
		parameterRoot := selected.root
		parameterFile := selected.file
		if refstr, ok := parameter.Get("$ref").AsStr(); ok {
			resolved, ok := resolveRef(refstr, document, ctx, LoaderJSONOrYAML)
			if !ok {
				continue
			}
			parameter = resolved.Node
			parameterRoot = resolved.Root
			parameterFile = resolved.File
		}
		requestFields = append(requestFields, openAPIParameterFields(parameter, ctx, parameterRoot, parameterFile, refs)...)
	}
	if requestBody := selected.operation.Get("requestBody"); requestBody != nil && requestBody.Kind != nullNode {
		body := requestBody
		bodyRoot := selected.root
		bodyFile := selected.file
		if refstr, ok := requestBody.Get("$ref").AsStr(); ok {
			resolved, ok := resolveRef(refstr, document, ctx, LoaderJSONOrYAML)
			if ok {
				body = resolved.Node
				bodyRoot = resolved.Root
				bodyFile = resolved.File
			} else {
				body = nil
			}
		}
		if body != nil && body.IsArrayLike() {
			requestFields = append(requestFields, openAPIBodyFields(body, ctx, bodyRoot, bodyFile, refs)...)
		}
	}

	status := ""
	responseSchema := Object()
	responseRoot := selected.root
	responseFile := selected.file
	if responses := selected.operation.Get("responses"); responses != nil && responses.IsArrayLike() {
		for _, responseEntry := range entries(responses) {
			if !strings.HasPrefix(responseEntry.Key, "2") {
				continue
			}
			response := responseEntry.Value
			currentRoot := selected.root
			currentFile := selected.file
			if refstr, ok := response.Get("$ref").AsStr(); ok {
				resolved, ok := resolveRef(refstr, document, ctx, LoaderJSONOrYAML)
				if ok {
					response = resolved.Node
					currentRoot = resolved.Root
					currentFile = resolved.File
				} else {
					response = nil
				}
			}
			if response != nil && response.IsArrayLike() {
				status = responseEntry.Key
				responseSchema = openAPIResponseSchema(response)
				responseRoot = currentRoot
				responseFile = currentFile
				break
			}
		}
	}
	if responseSchema.IsArrayLike() && !responseSchema.IsEmpty() {
		responseSchema = expandSchema(responseSchema, responseRoot, ctx, LoaderJSONOrYAML, responseFile)
	}
	responseFields := SchemaFields(responseSchema, ctx, refs, 0)
	responseName := "Response"
	if status != "" {
		responseName = "Response " + status
	}
	messages := []map[string]any{
		newMessage("Request", "request", requestFields),
		newMessage(responseName, "response", responseFields),
	}

	if sample := ctx.Options().Sample; sample != "" {
		direction := ctx.Options().Direction
		if direction == "" {
			direction = "response"
		}
		target := 1
		if direction == "request" {
			target = 0
		}
		var schema *Node
		if target == 1 && !responseSchema.IsEmpty() {
			schema = responseSchema
		}
		fields, _ := messages[target]["fields"].([]map[string]any)
		messages[target]["sampleValidation"] = sampleValidationFor(sample, schema, fields, refs, ctx)
	}

	operation := selected.signature()
	if selected.operationID != "" {
		operation = selected.operationID
	}
	return Parsed{Operation: operation, Messages: messages, Meta: map[string]any{}}, nil
}

func decodeOpenAPI(content string) (*Node, *ParseError) {
	trimmed := trimDocStart(content)
	if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
		node, message := ParseJSON(content)
		if message != "" {
			return nil, &ParseError{Message: "OpenAPI JSON parse error: " + message}
		}
		return node, nil
	}
	return parseYAML(content)
}

func collectOpenAPIOperations(paths, document *Node, ctx *Context) []openAPIOperation {
	var result []openAPIOperation
	for _, pathEntry := range entries(paths) {
		pathItem := pathEntry.Value
		if !pathItem.IsArrayLike() {
			continue
		}
		pathItemRoot := document
		pathItemFile := ctx.File()
		if refstr, ok := pathItem.Get("$ref").AsStr(); ok {
			if resolved, ok := resolveRef(refstr, document, ctx, LoaderJSONOrYAML); ok {
				pathItem = resolved.Node
				pathItemRoot = resolved.Root
				pathItemFile = resolved.File
			}
		}
		for _, method := range openAPIMethods {
			operation := pathItem.Get(method)
			if operation == nil || !operation.IsArrayLike() {
				continue
			}
			result = append(result, openAPIOperation{
				path:        pathEntry.Key,
				method:      method,
				operationID: operation.Get("operationId").ScalarString(),
				operation:   operation,
				pathItem:    pathItem,
				root:        pathItemRoot,
				file:        pathItemFile,
			})
		}
	}
	return result
}

func selectOpenAPIOperation(operations []openAPIOperation, wanted string, ctx *Context) *openAPIOperation {
	if wanted != "" {
		for i := range operations {
			if operations[i].operationID != "" && operations[i].operationID == wanted {
				return &operations[i]
			}
			// PHP only case-folds the "METHOD path" signature, never operationId.
			if strings.EqualFold(operations[i].signature(), wanted) {
				return &operations[i]
			}
		}
		ctx.Error("operation not found: "+wanted+"; candidates: "+openAPICandidates(operations), 0, 0)
		return &operations[0]
	}
	if len(operations) == 1 {
		return &operations[0]
	}
	ctx.Error("operation required; candidates: "+openAPICandidates(operations), 0, 0)
	return &operations[0]
}

func openAPICandidates(operations []openAPIOperation) string {
	names := make([]string, len(operations))
	for i, operation := range operations {
		names[i] = operation.displayName()
	}
	return strings.Join(names, ", ")
}

func openAPIParameterFields(parameter *Node, ctx *Context, root *Node, file string, refs *RefCtx) []map[string]any {
	name := parameter.Get("name").ScalarString()
	in := "query"
	if value := parameter.Get("in"); value != nil && value.Kind != nullNode {
		in = value.ScalarString()
	}
	if name == "" {
		return nil
	}
	schema := parameter.Get("schema")
	if schema == nil || !schema.IsArrayLike() {
		schema = parameter.Clone()
	}
	schema = expandSchema(schema, root, ctx, LoaderJSONOrYAML, file)
	desc := ""
	if value := parameter.Get("description"); value != nil && value.Kind != nullNode {
		desc = value.ScalarString()
	}
	field := map[string]any{
		"name":    name,
		"type":    SchemaTypeOf(schema),
		"require": parameter.Get("required").Truthy(),
		"desc":    desc,
		"in":      in,
	}
	children := SchemaFields(schema, ctx, refs, 0)
	if in == "body" {
		if len(children) == 0 {
			return []map[string]any{field}
		}
		return children
	}
	if len(children) > 0 {
		field["children"] = children
	}
	return []map[string]any{field}
}

func openAPIBodyFields(requestBody *Node, ctx *Context, root *Node, file string, refs *RefCtx) []map[string]any {
	content := requestBody.Get("content")
	if content == nil || !content.IsArrayLike() {
		return nil
	}
	schema := Object()
	for _, mediaType := range openAPIContentTypes {
		item := content.Get(mediaType)
		if item == nil {
			continue
		}
		if candidate := item.Get("schema"); candidate != nil && candidate.IsArrayLike() {
			schema = candidate
			break
		}
	}
	if !schema.IsArrayLike() || schema.IsEmpty() {
		return nil
	}
	schema = expandSchema(schema, root, ctx, LoaderJSONOrYAML, file)
	fields := SchemaFields(schema, ctx, refs, 0)
	for _, field := range fields {
		if _, ok := field["in"]; !ok {
			field["in"] = "body"
		}
	}
	return fields
}

func openAPIResponseSchema(response *Node) *Node {
	if content := response.Get("content"); content != nil && content.IsArrayLike() {
		for _, mediaType := range openAPIContentTypes {
			item := content.Get(mediaType)
			if item == nil {
				continue
			}
			if schema := item.Get("schema"); schema != nil && schema.IsArrayLike() {
				return schema
			}
		}
	}
	if schema := response.Get("schema"); schema != nil && schema.IsArrayLike() {
		return schema
	}
	return Object()
}

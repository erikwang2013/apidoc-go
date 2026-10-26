package protocol

func parseJSONSchema(ctx *Context, content string) (Parsed, *ParseError) {
	root, message := ParseJSON(content)
	if message != "" {
		return emptyParsed(), &ParseError{Message: "JSON parse error: " + message}
	}
	if !root.IsArrayLike() {
		return emptyParsed(), &ParseError{Message: "JSON parse error: root must be an object"}
	}
	messageName := ctx.Options().Message
	schema := root
	if messageName != "" {
		selected := root.Get("$defs").Get(messageName)
		if selected == nil || !selected.IsArrayLike() {
			selected = root.Get("definitions").Get(messageName)
		}
		if selected != nil && selected.IsArrayLike() {
			schema = selected
		} else {
			ctx.Error("message not found: "+messageName, 0, 0)
		}
	}
	refs := refsFor(root, LoaderJSON)
	expanded := expandSchema(schema, root, ctx, LoaderJSON, "")
	name := messageName
	if name == "" {
		if title := schema.Get("title"); title != nil && title.Kind != nullNode {
			name = title.ScalarString()
		} else {
			name = "Body"
		}
	}
	direction := ctx.Options().Direction
	if direction == "" {
		direction = "request"
	}
	sample := ctx.Options().Sample
	item := map[string]any{
		"name":             name,
		"direction":        direction,
		"fields":           SchemaFields(expanded, ctx, refs, 0),
		"sample":           sample,
		"sampleValidation": jsonSchemaSampleValidation(sample, expanded, refs, ctx),
	}
	meta := map[string]any{}
	if signature := schema.Get("x-webhook-signature"); signature != nil && signature.IsArrayLike() {
		// Divergence from PHP: it returns any array here, but the UIC contract
		// requires Definition.meta to be an object, so list signatures are ignored.
		if object, ok := signature.ToAny().(map[string]any); ok {
			meta = object
		}
	}
	return Parsed{Operation: "", Messages: []map[string]any{item}, Meta: meta}, nil
}

func jsonSchemaSampleValidation(sample string, schema *Node, refs *RefCtx, ctx *Context) SampleValidation {
	if sample == "" {
		return okSampleValidation()
	}
	data, message := ParseJSON(sample)
	if message != "" {
		return SampleValidation{
			OK: false,
			Errors: []SampleError{{
				Path: "$", Severity: "error", Message: "JSON parse error: " + message,
			}},
			Warnings: []SampleError{},
		}
	}
	return sampleValidationFromFailures(ValidateSchema(schema, data, refs, ctx, "$", 0))
}

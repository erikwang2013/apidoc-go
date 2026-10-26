package protocol

import (
	"strings"
)

var edmTypes = [][2]string{
	{"Edm.Int32", "integer"}, {"Edm.Int64", "integer"}, {"Edm.Int16", "integer"},
	{"Edm.Byte", "integer"}, {"Edm.String", "string"}, {"Edm.Boolean", "boolean"},
	{"Edm.Decimal", "number"}, {"Edm.Double", "number"}, {"Edm.Single", "number"},
	{"Edm.DateTimeOffset", "string"}, {"Edm.Guid", "string"},
}

func parseEDMX(ctx *Context, content string) (Parsed, *ParseError) {
	document, err := LoadXML(content)
	if err != nil {
		return emptyParsed(), err
	}
	entities := Object()
	complexTypes := Object()
	entitySets := Object()
	functions := Object()

	for _, entityType := range xmlDescendants(document.Root, "EntityType") {
		entities.Set(entityType.Attr("Name"), edmxProperties(entityType))
	}
	for _, complexType := range xmlDescendants(document.Root, "ComplexType") {
		complexTypes.Set(complexType.Attr("Name"), edmxProperties(complexType))
	}
	for _, entitySet := range xmlDescendants(document.Root, "EntitySet") {
		entitySets.Set(entitySet.Attr("Name"), StringNode(edmxLocalName(entitySet.Attr("EntityType"))))
	}
	for _, function := range xmlDescendants(document.Root, "FunctionImport") {
		fields := ArrayNode()
		for _, parameter := range function.All("Parameter") {
			field := Object()
			field.Set("name", StringNode(parameter.Attr("Name")))
			field.Set("type", StringNode(edmxType(parameter.Attr("Type"))))
			field.Set("require", BoolNode(true))
			fields.Push(field)
		}
		functions.Set(function.Attr("Name"), fields)
	}

	var candidates []string
	for _, entry := range entries(entitySets) {
		candidates = append(candidates, entry.Key)
	}
	for _, entry := range entries(functions) {
		candidates = append(candidates, entry.Key)
	}
	wanted := ctx.Options().Operation
	if wanted != "" && !containsString(candidates, wanted) {
		ctx.Error("operation not found: "+wanted+"; candidates: "+strings.Join(candidates, ", "), 0, 0)
		return emptyParsed(), nil
	}
	if wanted == "" {
		if len(candidates) == 1 {
			wanted = candidates[0]
		} else {
			ctx.Error("operation required; candidates: "+strings.Join(candidates, ", "), 0, 0)
			return emptyParsed(), nil
		}
	}

	var fields []*Node
	direction := ""
	if entityName := entitySets.Get(wanted); entityName != nil {
		entity := entityName.ScalarString()
		source := entities.Get(entity)
		if source == nil {
			source = ArrayNode()
		}
		fields = edmxResolveFields(source, entities, complexTypes, nil, ctx, newExpansionBudget())
		direction = "response"
	} else {
		source := functions.Get(wanted)
		if source == nil {
			source = ArrayNode()
		}
		for _, entry := range entries(source) {
			fields = append(fields, entry.Value)
		}
		direction = "request"
	}
	messageFields := make([]map[string]any, len(fields))
	for i, field := range fields {
		messageFields[i] = field.ToAny().(map[string]any)
	}
	messages := []map[string]any{newMessage(wanted, direction, messageFields)}
	if sample := ctx.Options().Sample; sample != "" {
		messages[0]["sampleValidation"] = validateEDMXSample(sample, messageFields, ctx)
	}
	return Parsed{
		Operation: wanted,
		Messages:  messages,
		Meta:      map[string]any{"direction": direction},
	}, nil
}

func edmxProperties(node *XMLNode) *Node {
	fields := ArrayNode()
	for _, property := range node.All("Property") {
		field := Object()
		field.Set("name", StringNode(property.Attr("Name")))
		field.Set("type", StringNode(edmxType(property.Attr("Type"))))
		field.Set("require", BoolNode(property.Attr("Nullable") == "false"))
		fields.Push(field)
	}
	for _, navigation := range node.All("NavigationProperty") {
		field := Object()
		field.Set("name", StringNode(navigation.Attr("Name")))
		field.Set("type", StringNode("object"))
		field.Set("require", BoolNode(false))
		field.Set("ref", StringNode(edmxLocalName(navigation.Attr("Type"))))
		fields.Push(field)
	}
	return fields
}

func edmxResolveFields(fields, entities, complexTypes *Node, visited []string, ctx *Context, budget *expansionBudget) []*Node {
	result := []*Node{}
	for _, entry := range entries(fields) {
		if !budget.take(ctx) {
			break
		}
		field := entry.Value
		reference := field.Get("ref").ScalarString()
		if reference != "" && !containsString(visited, reference) {
			// PHP appends to its copy-on-write local, so later siblings see it.
			visited = append(visited, reference)
			target := entities.Get(reference)
			if target == nil {
				target = complexTypes.Get(reference)
			}
			if target == nil {
				target = ArrayNode()
			}
			field.Set("children", arrayFromNodes(edmxResolveFields(target, entities, complexTypes, visited, ctx, budget)))
		}
		result = append(result, field)
	}
	return result
}

func validateEDMXSample(sample string, fields []map[string]any, ctx *Context) SampleValidation {
	data, message := ParseJSON(sample)
	if message != "" {
		return SampleValidation{
			OK: false,
			Errors: []SampleError{{
				Path: "$", Severity: "error", Message: "JSON sample parse error: " + message,
			}},
			Warnings: []SampleError{},
		}
	}
	schema := edmxSchemaForFields(fields)
	refs := NewRefCtx(Object(), "", LoaderJSONOrYAML)
	failures := ValidateSchema(schema, data, &refs, ctx, "$", 0)
	errors := make([]SampleError, len(failures))
	for i, failure := range failures {
		errors[i] = SampleError{Path: failure.Path, Severity: "error", Message: failure.Message}
	}
	return SampleValidation{OK: len(errors) == 0, Errors: errors, Warnings: []SampleError{}}
}

func edmxSchemaForFields(fields []map[string]any) *Node {
	properties := Object()
	required := ArrayNode()
	for _, field := range fields {
		name, _ := field["name"].(string)
		fieldType, _ := field["type"].(string)
		if fieldType == "" {
			fieldType = "string"
		}
		property := Object()
		property.Set("type", StringNode(edmxJSONType(fieldType)))
		if enumValues, ok := field["enum"].([]any); ok && len(enumValues) > 0 {
			values := ArrayNode()
			for _, value := range enumValues {
				if text, ok := value.(string); ok {
					values.Push(StringNode(text))
				}
			}
			property.Set("enum", values)
		}
		if fieldType == "array" {
			childrenType, _ := field["childrenType"].(string)
			if childrenType == "" {
				childrenType = "string"
			}
			items := Object()
			items.Set("type", StringNode(edmxJSONType(childrenType)))
			property.Set("items", items)
		}
		if fieldType == "object" {
			property = Object()
			property.Set("type", StringNode("object"))
			property.Set("properties", Object())
		}
		properties.Set(name, property)
		if require, _ := field["require"].(bool); require {
			required.Push(StringNode(name))
		}
	}
	schema := Object()
	schema.Set("type", StringNode("object"))
	schema.Set("properties", properties)
	schema.Set("required", required)
	return schema
}

func edmxJSONType(fieldType string) string {
	switch fieldType {
	case "integer", "number", "boolean", "array", "object", "string":
		return fieldType
	}
	return "string"
}

func edmxType(fieldType string) string {
	if strings.HasPrefix(fieldType, "Collection(") {
		return "array"
	}
	for _, entry := range edmTypes {
		if entry[0] == fieldType {
			return entry[1]
		}
	}
	return "string"
}

func edmxLocalName(qualified string) string {
	if index := strings.LastIndexByte(qualified, '.'); index >= 0 {
		return qualified[index+1:]
	}
	return qualified
}

package protocol

import (
	"fmt"
	"strings"
)

var graphQLDefinitions = map[string]bool{
	"schema": true, "type": true, "input": true, "interface": true, "enum": true,
	"scalar": true, "union": true, "extend": true, "directive": true,
}

var graphQLScalars = map[string]string{
	"String": "string", "ID": "string", "Int": "integer", "Float": "number", "Boolean": "boolean",
}

func parseGraphQL(ctx *Context, content string) (Parsed, *ParseError) {
	types := Object()
	if err := parseGraphQLDocument(ctx, content, types); err != nil {
		return emptyParsed(), err
	}
	queryType := "Query"
	if node := types.Get("__schema").Get("query"); node != nil {
		queryType = node.ScalarString()
	}
	queryFields := types.Get(queryType).Get("fields")
	if queryFields == nil || !queryFields.IsArrayLike() {
		queryFields = Object()
	}
	typeName, fieldName, field, ok := selectGraphQL(types, queryType, queryFields, ctx.Options().Operation, ctx)
	if !ok {
		return emptyParsed(), nil
	}
	request := []map[string]any{}
	if args, ok := field.Get("args").AsArray(); ok {
		for _, arg := range args {
			typeRef := arg.Get("type")
			if typeRef == nil {
				continue
			}
			descriptor := describeGraphQLType(typeRef, types, nil, 0)
			descriptor.Set("name", StringNode(arg.Get("name").ScalarString()))
			descriptor.Set("require", BoolNode(typeRef.Get("require").Truthy()))
			request = append(request, descriptor.ToAny().(map[string]any))
		}
	}
	responseNode := ArrayNode()
	if typeRef := field.Get("type"); typeRef != nil {
		responseNode = graphQLResponseFields(typeRef, types)
	}
	response := []map[string]any{}
	for _, item := range responseNode.Arr {
		response = append(response, item.ToAny().(map[string]any))
	}
	messages := []map[string]any{
		newMessage("Request", "request", request),
		newMessage("Response", "response", response),
	}
	if sample := ctx.Options().Sample; sample != "" {
		target := 1
		if ctx.Options().Direction == "request" {
			target = 0
		}
		fields, _ := messages[target]["fields"].([]map[string]any)
		messages[target]["sampleValidation"] = validateGraphQLSample(sample, fields)
	}
	return Parsed{
		Operation: typeName + "." + fieldName,
		Messages:  messages,
		Meta:      map[string]any{},
	}, nil
}

func parseGraphQLDocument(ctx *Context, content string, types *Node) *ParseError {
	for _, importPath := range graphQLImports(content) {
		realpath, ok := ctx.Resolve(importPath)
		if !ok {
			ctx.Warning("unable to resolve GraphQL import: "+importPath, 0, 0)
			continue
		}
		if !ctx.Push(realpath) {
			continue
		}
		imported, read := ctx.Read()
		if read {
			importedTypes := Object()
			if err := parseGraphQLDocument(ctx, imported, importedTypes); err != nil {
				ctx.Pop()
				return err
			}
			for _, member := range importedTypes.Obj {
				types.Set(member.Key, member.Value)
			}
		}
		ctx.Pop()
	}
	ts := NewTokenStream(content)
	for !ts.EOF() {
		skipGraphQLDescription(ts)
		if ts.EOF() {
			break
		}
		keyword, err := ts.Identifier()
		if err != nil {
			return err
		}
		switch keyword {
		case "schema":
			if err := parseGraphQLSchema(ts, types); err != nil {
				return err
			}
		case "type", "interface", "input":
			if err := parseGraphQLTypeDefinition(ts, types, keyword, false); err != nil {
				return err
			}
		case "enum":
			if err := parseGraphQLEnum(ts, types); err != nil {
				return err
			}
		case "scalar":
			name, err := ts.Identifier()
			if err != nil {
				return err
			}
			entry := Object()
			entry.Set("kind", StringNode("scalar"))
			types.Set(name, entry)
		case "union":
			if err := parseGraphQLUnion(ts, types); err != nil {
				return err
			}
		case "extend":
			kind, err := ts.Identifier()
			if err != nil {
				return err
			}
			if kind == "type" || kind == "interface" || kind == "input" {
				if err := parseGraphQLTypeDefinition(ts, types, kind, true); err != nil {
					return err
				}
			} else {
				skipGraphQLDefinition(ts)
			}
		case "directive":
			skipGraphQLDirective(ts)
		default:
			ctx.Warning("unsupported GraphQL definition: "+keyword, 0, 0)
			skipGraphQLDefinition(ts)
		}
	}
	return nil
}

func graphQLImports(content string) []string {
	var result []string
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimLeft(line, " \t\r\n\f\v")
		rest, ok := strings.CutPrefix(trimmed, "#import")
		if !ok || rest == "" || (rest[0] != ' ' && rest[0] != '\t') {
			continue
		}
		rest = strings.TrimLeft(rest, " \t")
		if rest == "" || (rest[0] != '"' && rest[0] != '\'') {
			continue
		}
		quote := rest[0]
		if end := strings.IndexByte(rest[1:], quote); end >= 0 {
			result = append(result, rest[1:1+end])
		}
	}
	return result
}

func parseGraphQLSchema(ts *TokenStream, types *Node) *ParseError {
	if err := ts.Expect('{'); err != nil {
		return err
	}
	for !ts.EOF() && ts.Peek(0) != "}" {
		skipGraphQLDescription(ts)
		if ts.Peek(0) == "}" {
			break
		}
		kind, err := ts.Identifier()
		if err != nil {
			return err
		}
		if err := ts.Expect(':'); err != nil {
			return err
		}
		value, err := ts.Identifier()
		if err != nil {
			return err
		}
		if !types.Get("__schema").IsObject() {
			types.Set("__schema", Object())
		}
		types.Get("__schema").Set(kind, StringNode(value))
		ts.Consume(",")
	}
	return ts.Expect('}')
}

func parseGraphQLTypeDefinition(ts *TokenStream, types *Node, kind string, extend bool) *ParseError {
	name, err := ts.Identifier()
	if err != nil {
		return err
	}
	for !ts.EOF() && ts.Peek(0) != "{" {
		if ts.Peek(0) == "}" {
			break
		}
		ts.NextChar()
	}
	fields, err := parseGraphQLFields(ts)
	if err != nil {
		return err
	}
	if extend && types.Get(name).Get("fields") != nil {
		existing := types.Get(name).Get("fields")
		if existing == nil {
			existing = Object()
		}
		types.Get(name).Set("fields", Merge(existing, fields))
	} else {
		entry := Object()
		entry.Set("kind", StringNode(kind))
		entry.Set("fields", fields)
		types.Set(name, entry)
	}
	return nil
}

func parseGraphQLFields(ts *TokenStream) (*Node, *ParseError) {
	fields := Object()
	if err := ts.Expect('{'); err != nil {
		return nil, err
	}
	for !ts.EOF() && ts.Peek(0) != "}" {
		skipGraphQLDescription(ts)
		if ts.Peek(0) == "}" {
			break
		}
		name, err := ts.Identifier()
		if err != nil {
			return nil, err
		}
		args := ArrayNode()
		if _, ok := ts.Consume("("); ok {
			args, err = parseGraphQLArguments(ts)
			if err != nil {
				return nil, err
			}
		}
		if err := ts.Expect(':'); err != nil {
			return nil, err
		}
		typeRef, err := parseGraphQLTypeRef(ts, 0)
		if err != nil {
			return nil, err
		}
		skipGraphQLDirectives(ts)
		field := Object()
		field.Set("name", StringNode(name))
		field.Set("type", typeRef)
		field.Set("args", args)
		fields.Set(name, field)
	}
	return fields, ts.Expect('}')
}

func parseGraphQLArguments(ts *TokenStream) (*Node, *ParseError) {
	args := ArrayNode()
	for !ts.EOF() && ts.Peek(0) != ")" {
		skipGraphQLDescription(ts)
		if ts.Peek(0) == ")" {
			break
		}
		name, err := ts.Identifier()
		if err != nil {
			return nil, err
		}
		if err := ts.Expect(':'); err != nil {
			return nil, err
		}
		typeRef, err := parseGraphQLTypeRef(ts, 0)
		if err != nil {
			return nil, err
		}
		if _, ok := ts.Consume("="); ok {
			skipGraphQLDefaultValue(ts)
		}
		arg := Object()
		arg.Set("name", StringNode(name))
		arg.Set("type", typeRef)
		args.Push(arg)
		ts.Consume(",")
	}
	return args, ts.Expect(')')
}

func parseGraphQLTypeRef(ts *TokenStream, depth int) (*Node, *ParseError) {
	if depth > 256 {
		return nil, ts.Errorf("graphql nesting limit reached")
	}
	var typeRef *Node
	if _, ok := ts.Consume("["); ok {
		inner, err := parseGraphQLTypeRef(ts, depth+1)
		if err != nil {
			return nil, err
		}
		if err := ts.Expect(']'); err != nil {
			return nil, err
		}
		typeRef = Object()
		typeRef.Set("kind", StringNode("array"))
		typeRef.Set("inner", inner)
	} else {
		name, err := ts.Identifier()
		if err != nil {
			return nil, err
		}
		typeRef = Object()
		typeRef.Set("kind", StringNode("named"))
		typeRef.Set("name", StringNode(name))
	}
	_, required := ts.Consume("!")
	typeRef.Set("require", BoolNode(required))
	return typeRef, nil
}

func parseGraphQLEnum(ts *TokenStream, types *Node) *ParseError {
	name, err := ts.Identifier()
	if err != nil {
		return err
	}
	values := ArrayNode()
	if err := ts.Expect('{'); err != nil {
		return err
	}
	for !ts.EOF() && ts.Peek(0) != "}" {
		skipGraphQLDescription(ts)
		if ts.Peek(0) == "}" {
			break
		}
		value, err := ts.Identifier()
		if err != nil {
			return err
		}
		values.Push(StringNode(value))
		ts.Consume(",")
	}
	if err := ts.Expect('}'); err != nil {
		return err
	}
	entry := Object()
	entry.Set("kind", StringNode("enum"))
	entry.Set("values", values)
	types.Set(name, entry)
	return nil
}

func parseGraphQLUnion(ts *TokenStream, types *Node) *ParseError {
	name, err := ts.Identifier()
	if err != nil {
		return err
	}
	members := ArrayNode()
	if _, ok := ts.Consume("="); ok {
		for !ts.EOF() {
			member, err := ts.Identifier()
			if err != nil {
				return err
			}
			members.Push(StringNode(member))
			if _, ok := ts.Consume("|"); !ok {
				break
			}
		}
	}
	entry := Object()
	entry.Set("kind", StringNode("union"))
	entry.Set("members", members)
	types.Set(name, entry)
	return nil
}

func selectGraphQL(types *Node, queryType string, queryFields *Node, wanted string, ctx *Context) (string, string, *Node, bool) {
	if wanted != "" {
		typeName, fieldName := queryType, wanted
		if index := strings.LastIndexByte(wanted, '.'); index >= 0 {
			typeName, fieldName = wanted[:index], wanted[index+1:]
		}
		if definition := types.Get(typeName).Get("fields").Get(fieldName); definition != nil {
			return typeName, fieldName, definition, true
		}
		ctx.Error("operation not found: "+wanted+"; candidates: "+graphQLCandidates(queryType, queryFields), 0, 0)
		return "", "", nil, false
	}
	fieldEntries := entries(queryFields)
	if len(fieldEntries) == 1 {
		return queryType, fieldEntries[0].Key, fieldEntries[0].Value, true
	}
	ctx.Error("operation required; candidates: "+graphQLCandidates(queryType, queryFields), 0, 0)
	return "", "", nil, false
}

func graphQLCandidates(typeName string, fields *Node) string {
	fieldEntries := entries(fields)
	names := make([]string, len(fieldEntries))
	for i, entry := range fieldEntries {
		names[i] = typeName + "." + entry.Key
	}
	return strings.Join(names, ", ")
}

func graphQLResponseFields(typeRef, types *Node) *Node {
	kind := typeRef.Get("kind").ScalarString()
	if kind == "array" {
		inner := Object()
		if node := typeRef.Get("inner"); node != nil {
			inner = describeGraphQLType(node, types, nil, 0)
		}
		field := Object()
		field.Set("name", StringNode("items"))
		field.Set("type", StringNode("array"))
		field.Set("require", BoolNode(typeRef.Get("require").Truthy()))
		children := inner.Get("children")
		if children == nil {
			children = ArrayNode()
		}
		field.Set("children", children)
		result := ArrayNode()
		result.Push(field)
		return result
	}
	if kind == "named" {
		descriptor := describeGraphQLType(typeRef, types, nil, 0)
		descriptorType := descriptor.Get("type").ScalarString()
		if descriptorType == "object" {
			children := descriptor.Get("children")
			if children == nil {
				children = ArrayNode()
			}
			return children
		}
		field := Object()
		field.Set("name", StringNode("result"))
		field.Set("type", StringNode(descriptorType))
		field.Set("require", BoolNode(typeRef.Get("require").Truthy()))
		result := ArrayNode()
		result.Push(field)
		return result
	}
	return ArrayNode()
}

func describeGraphQLType(typeRef, types *Node, visited []string, depth int) *Node {
	if depth > 256 {
		node := Object()
		node.Set("type", StringNode("object"))
		node.Set("children", ArrayNode())
		return node
	}
	kind := typeRef.Get("kind").ScalarString()
	require := typeRef.Get("require").Truthy()
	if kind == "array" {
		inner := Object()
		if node := typeRef.Get("inner"); node != nil {
			inner = describeGraphQLType(node, types, visited, depth+1)
		}
		result := Object()
		result.Set("type", StringNode("array"))
		result.Set("require", BoolNode(require))
		if inner.Get("type").ScalarString() == "object" {
			children := inner.Get("children")
			if children == nil {
				children = ArrayNode()
			}
			result.Set("children", children)
			result.Set("childrenType", StringNode("object"))
		} else {
			childrenType := inner.Get("type").ScalarString()
			if childrenType == "" {
				childrenType = "string"
			}
			result.Set("childrenType", StringNode(childrenType))
		}
		return result
	}
	name := typeRef.Get("name").ScalarString()
	if scalar, ok := graphQLScalars[name]; ok {
		result := Object()
		result.Set("type", StringNode(scalar))
		result.Set("require", BoolNode(require))
		return result
	}
	if types.Get(name).Get("kind").ScalarString() == "enum" {
		result := Object()
		result.Set("type", StringNode("string"))
		result.Set("require", BoolNode(require))
		values := types.Get(name).Get("values")
		if values == nil {
			values = ArrayNode()
		}
		result.Set("enum", values)
		return result
	}
	hasFields := types.Get(name).Get("fields") != nil
	if hasFields && !containsString(visited, name) {
		nextVisited := append(append([]string{}, visited...), name)
		children := ArrayNode()
		if fields := types.Get(name).Get("fields"); fields != nil {
			children = mapGraphQLFields(fields, types, nextVisited, depth+1)
		}
		result := Object()
		result.Set("type", StringNode("object"))
		result.Set("require", BoolNode(require))
		result.Set("ref", StringNode(name))
		result.Set("children", children)
		return result
	}
	result := Object()
	result.Set("type", StringNode("object"))
	result.Set("require", BoolNode(require))
	result.Set("ref", StringNode(name))
	result.Set("children", ArrayNode())
	return result
}

func mapGraphQLFields(fields, types *Node, visited []string, depth int) *Node {
	result := ArrayNode()
	for _, fieldEntry := range entries(fields) {
		field := fieldEntry.Value
		typeRef := field.Get("type")
		if typeRef == nil {
			continue
		}
		descriptor := describeGraphQLType(typeRef, types, visited, depth)
		descriptor.Set("name", StringNode(field.Get("name").ScalarString()))
		result.Push(descriptor)
	}
	return result
}

func validateGraphQLSample(sample string, fields []map[string]any) SampleValidation {
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
	errors := []SampleError{}
	validateGraphQLFields(fields, data, "$", &errors)
	return SampleValidation{OK: len(errors) == 0, Errors: errors, Warnings: []SampleError{}}
}

func validateGraphQLFields(fields []map[string]any, data *Node, path string, errors *[]SampleError) {
	if !data.IsArrayLike() {
		*errors = append(*errors, SampleError{Path: path, Severity: "error", Message: "expected object"})
		return
	}
	for _, field := range fields {
		name, _ := field["name"].(string)
		value := data.Get(name)
		if value == nil {
			require, _ := field["require"].(bool)
			if require {
				*errors = append(*errors, SampleError{
					Path: path + "." + name, Severity: "error", Message: "required field missing",
				})
			}
			continue
		}
		fieldType, _ := field["type"].(string)
		children, _ := field["children"].([]map[string]any)
		fieldPath := path + "." + name
		switch {
		case fieldType == "object" && len(children) > 0:
			validateGraphQLFields(children, value, fieldPath, errors)
		case fieldType == "array" && len(children) > 0:
			if !value.IsArrayLike() {
				*errors = append(*errors, SampleError{Path: fieldPath, Severity: "error", Message: "expected array"})
				continue
			}
			for _, item := range entries(value) {
				validateGraphQLFields(children, item.Value, fmt.Sprintf("%s[%s]", fieldPath, item.Key), errors)
			}
		case fieldType == "integer" && value.Kind != intNode:
			*errors = append(*errors, SampleError{Path: fieldPath, Severity: "error", Message: "expected integer"})
		case fieldType == "number" && value.Kind != intNode && value.Kind != floatNode:
			*errors = append(*errors, SampleError{Path: fieldPath, Severity: "error", Message: "expected number"})
		case fieldType == "boolean" && value.Kind != boolNode:
			*errors = append(*errors, SampleError{Path: fieldPath, Severity: "error", Message: "expected boolean"})
		case fieldType == "string" && value.Kind != stringNode:
			*errors = append(*errors, SampleError{Path: fieldPath, Severity: "error", Message: "expected string"})
		}
	}
}

func skipGraphQLDescription(ts *TokenStream) {
	if ts.Peek(0) != "\"" {
		return
	}
	if ts.Peek(1) == "\"" && ts.Peek(2) == "\"" {
		ts.NextChar()
		ts.NextChar()
		ts.NextChar()
		for !ts.EOF() {
			if ts.Peek(0) == "\"" && ts.Peek(1) == "\"" && ts.Peek(2) == "\"" {
				ts.NextChar()
				ts.NextChar()
				ts.NextChar()
				return
			}
			ts.NextChar()
		}
		return
	}
	_, _ = ts.String()
}

func skipGraphQLDirectives(ts *TokenStream) {
	for {
		if _, ok := ts.Consume("@"); !ok {
			return
		}
		_, _ = ts.Identifier()
		if _, ok := ts.Consume("("); ok {
			skipGraphQLBalanced(ts, '(', ')')
		}
	}
}

func skipGraphQLDefaultValue(ts *TokenStream) {
	ch := ts.Peek(0)
	if ch == "[" {
		ts.NextChar()
		skipGraphQLBalanced(ts, '[', ']')
		return
	}
	if ch == "{" {
		ts.NextChar()
		skipGraphQLBalanced(ts, '{', '}')
		return
	}
	if ch == "\"" || ch == "'" {
		_, _ = ts.String()
		return
	}
	for !ts.EOF() {
		ch = ts.Peek(0)
		if ch == "," || ch == ")" || ch == "}" {
			return
		}
		ts.NextChar()
	}
}

func skipGraphQLBalanced(ts *TokenStream, open, close rune) {
	depth := 1
	for !ts.EOF() && depth > 0 {
		ch := ts.NextChar()
		if ch == string(open) {
			depth++
		} else if ch == string(close) {
			depth--
		}
	}
}

func skipGraphQLDirective(ts *TokenStream) {
	ts.Consume("@")
	if !ts.EOF() {
		_, _ = ts.Identifier()
	}
	if _, ok := ts.Consume("("); ok {
		skipGraphQLBalanced(ts, '(', ')')
	}
	if !ts.EOF() {
		_, _ = ts.Identifier()
	}
	for !ts.EOF() {
		ch := ts.Peek(0)
		if ch == "|" || (ch != "" && (isASCIIAlpha(ch[0]) || ch[0] == '_')) {
			ts.NextChar()
			continue
		}
		break
	}
}

func skipGraphQLDefinition(ts *TokenStream) {
	for !ts.EOF() {
		ch := ts.Peek(0)
		if ch == "}" {
			ts.NextChar()
			return
		}
		if ch != "" && (isASCIIAlpha(ch[0]) || ch[0] == '_') {
			word, err := ts.Identifier()
			if err != nil {
				return
			}
			if graphQLDefinitions[word] {
				return
			}
			continue
		}
		ts.NextChar()
	}
}

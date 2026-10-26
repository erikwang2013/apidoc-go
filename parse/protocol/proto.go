package protocol

import (
	"strconv"
	"strings"
)

var protoScalars = [][2]string{
	{"double", "number"},
	{"float", "number"},
	{"int32", "integer"},
	{"int64", "integer"},
	{"uint32", "integer"},
	{"uint64", "integer"},
	{"sint32", "integer"},
	{"sint64", "integer"},
	{"fixed32", "integer"},
	{"fixed64", "integer"},
	{"sfixed32", "integer"},
	{"sfixed64", "integer"},
	{"bool", "boolean"},
	{"string", "string"},
	{"bytes", "string"},
}

type protoRPC struct {
	service   string
	operation string
	rpc       *Node
}

func parseProto(ctx *Context, content string) (Parsed, *ParseError) {
	symbols, err := parseProtoDocument(ctx, content, 0)
	if err != nil {
		return emptyParsed(), err
	}
	packageName := symbols.Get("package").ScalarString()
	var rpcs []protoRPC
	if services := symbols.Get("services"); services.IsObject() {
		for _, serviceEntry := range entries(services) {
			servicePackage := serviceEntry.Value.Get("package").ScalarString()
			rpcList, _ := serviceEntry.Value.Get("rpcs").AsArray()
			for _, rpc := range rpcList {
				rpcName := rpc.Get("name").ScalarString()
				prefix := ""
				if servicePackage != "" {
					prefix = servicePackage + "."
				}
				rpcs = append(rpcs, protoRPC{
					service:   serviceEntry.Key,
					operation: prefix + serviceEntry.Key + "/" + rpcName,
					rpc:       rpc,
				})
			}
		}
	}
	selected := selectProtoRPC(rpcs, ctx.Options().Operation, ctx)
	if selected == nil {
		return emptyParsed(), nil
	}
	rpc := selected.rpc
	requestName := rpc.Get("request").ScalarString()
	responseName := rpc.Get("response").ScalarString()
	request := protoMessageForType(requestName, symbols, ctx, nil)
	response := protoMessageForType(responseName, symbols, ctx, nil)
	requestMessage := map[string]any{
		"name":         requestName,
		"direction":    "request",
		"fields":       request,
		"clientStream": rpc.Get("client_stream").Truthy(),
	}
	responseMessage := map[string]any{
		"name":         responseName,
		"direction":    "response",
		"fields":       response,
		"serverStream": rpc.Get("server_stream").Truthy(),
	}
	if ctx.Options().Sample != "" {
		ctx.Warning("protobuf sample validation is planned for P3", 0, 0)
	}
	return Parsed{
		Operation: selected.operation,
		Messages:  []map[string]any{requestMessage, responseMessage},
		Meta:      map[string]any{"package": packageName},
	}, nil
}

func parseProtoDocument(ctx *Context, content string, depth int) (*Node, *ParseError) {
	if depth > 32 {
		return nil, &ParseError{Message: "proto nesting limit reached"}
	}
	symbols := protoSymbols()
	ts := NewTokenStream(content)
	for !ts.EOF() {
		line, column := ts.Line(), ts.Column()
		keyword, err := ts.Identifier()
		if err != nil {
			return nil, err
		}
		switch keyword {
		case "syntax":
			if err := ts.Expect('='); err != nil {
				return nil, err
			}
			if _, err := ts.String(); err != nil {
				return nil, err
			}
			if err := ts.Expect(';'); err != nil {
				return nil, err
			}
		case "package":
			name, err := ts.Identifier()
			if err != nil {
				return nil, err
			}
			symbols.Set("package", StringNode(name))
			if err := ts.Expect(';'); err != nil {
				return nil, err
			}
		case "import":
			importLine := ts.Line()
			next := ts.Peek(0)
			if next == "p" || next == "w" {
				if _, err := ts.Identifier(); err != nil {
					return nil, err
				}
			}
			file, err := ts.String()
			if err != nil {
				return nil, err
			}
			if err := ts.Expect(';'); err != nil {
				return nil, err
			}
			if err := protoImport(ctx, symbols, file, importLine, depth); err != nil {
				return nil, err
			}
		case "option":
			skipProtoToSemicolon(ts)
		case "message":
			if err := parseProtoMessage(ts, symbols, ctx, depth+1); err != nil {
				return nil, err
			}
		case "enum":
			if err := parseProtoEnum(ts, symbols); err != nil {
				return nil, err
			}
		case "service":
			if err := parseProtoService(ts, symbols, ctx, symbols.Get("package").ScalarString()); err != nil {
				return nil, err
			}
		case "extend", "group", "edition":
			ctx.Unsupported("unsupported construct: "+keyword, line, column)
			skipProtoDefinition(ts)
		case "reserved":
			skipProtoToSemicolon(ts)
		default:
			ctx.Warning("unsupported proto definition: "+keyword, 0, 0)
			skipProtoDefinition(ts)
		}
	}
	aliasProtoSymbols(symbols)
	return symbols, nil
}

func protoSymbols() *Node {
	symbols := Object()
	symbols.Set("messages", Object())
	symbols.Set("enums", Object())
	symbols.Set("services", Object())
	symbols.Set("package", StringNode(""))
	return symbols
}

func protoImport(ctx *Context, symbols *Node, file string, line int64, depth int) *ParseError {
	realpath, ok := ctx.Resolve(file)
	if !ok {
		ctx.Error("unable to resolve import: "+file, line, 0)
		return nil
	}
	if !ctx.Push(realpath) {
		return nil
	}
	content, read := ctx.Read()
	if read {
		imported, err := parseProtoDocument(ctx, content, depth+1)
		if err != nil {
			ctx.Pop()
			return err
		}
		for _, kind := range []string{"messages", "enums", "services"} {
			base := symbols.Get(kind)
			if base == nil {
				base = Object()
			}
			symbols.Set(kind, Merge(base, imported.Get(kind)))
		}
	}
	ctx.Pop()
	return nil
}

func aliasProtoSymbols(symbols *Node) {
	packageName := symbols.Get("package").ScalarString()
	if packageName == "" {
		return
	}
	for _, kind := range []string{"messages", "enums", "services"} {
		container := symbols.Get(kind)
		if container == nil || !container.IsObject() {
			continue
		}
		for _, entry := range append([]Entry{}, entries(container)...) {
			container.Set(packageName+"."+entry.Key, entry.Value)
		}
	}
}

func parseProtoMessage(ts *TokenStream, symbols *Node, ctx *Context, depth int) *ParseError {
	if depth > 128 {
		return ts.Errorf("proto nesting limit reached")
	}
	name, err := ts.Identifier()
	if err != nil {
		return err
	}
	if err := ts.Expect('{'); err != nil {
		return err
	}
	fields := ArrayNode()
	for !ts.EOF() && ts.Peek(0) != "}" {
		word := ts.Peek(0)
		if len(word) > 0 && (isASCIIAlpha(word[0]) || word == "_") {
			token, err := ts.Identifier()
			if err != nil {
				return err
			}
			switch token {
			case "message":
				if err := parseProtoMessage(ts, symbols, ctx, depth+1); err != nil {
					return err
				}
				continue
			case "enum":
				if err := parseProtoEnum(ts, symbols); err != nil {
					return err
				}
				continue
			case "oneof":
				oneofName, err := ts.Identifier()
				if err != nil {
					return err
				}
				if err := ts.Expect('{'); err != nil {
					return err
				}
				for !ts.EOF() && ts.Peek(0) != "}" {
					field, err := parseProtoField(ts, true, oneofName, depth)
					if err != nil {
						return err
					}
					if field != nil {
						fields.Push(field)
					}
				}
				if err := ts.Expect('}'); err != nil {
					return err
				}
				continue
			case "option", "reserved", "extensions":
				skipProtoToSemicolon(ts)
				continue
			}
			var field *Node
			if token == "repeated" || token == "optional" || token == "required" {
				field, err = parseProtoFieldAfterLabel(ts, "", false, "", token, depth)
			} else if token == "map" {
				field, err = parseProtoFieldAfterLabel(ts, "map", false, "", "", depth)
			} else {
				field, err = parseProtoFieldAfterLabel(ts, token, false, "", "", depth)
			}
			if err != nil {
				return err
			}
			if field != nil {
				fields.Push(field)
			}
			continue
		}
		ts.NextChar()
	}
	if err := ts.Expect('}'); err != nil {
		return err
	}
	entry := Object()
	entry.Set("name", StringNode(name))
	entry.Set("fields", fields)
	symbols.Get("messages").Set(name, entry)
	return nil
}

func parseProtoField(ts *TokenStream, inOneof bool, oneof string, depth int) (*Node, *ParseError) {
	label := ""
	peek := ts.Peek(0)
	if len(peek) > 0 && (isASCIIAlpha(peek[0]) || peek == "_") {
		word, err := ts.Identifier()
		if err != nil {
			return nil, err
		}
		if word == "repeated" || word == "optional" || word == "required" {
			label = word
		} else {
			return parseProtoFieldAfterLabel(ts, word, inOneof, oneof, "", depth)
		}
	}
	return parseProtoFieldAfterLabel(ts, "", inOneof, oneof, label, depth)
}

func parseProtoFieldAfterLabel(ts *TokenStream, firstType string, inOneof bool, oneof, label string, depth int) (*Node, *ParseError) {
	if depth > 128 {
		return nil, ts.Errorf("proto nesting limit reached")
	}
	isMap := false
	mapKey, mapValue := "", ""
	var fieldType string
	switch {
	case firstType == "map":
		isMap = true
		if err := ts.Expect('<'); err != nil {
			return nil, err
		}
		var err *ParseError
		mapKey, err = ts.Identifier()
		if err != nil {
			return nil, err
		}
		if err := ts.Expect(','); err != nil {
			return nil, err
		}
		mapValue, err = ts.Identifier()
		if err != nil {
			return nil, err
		}
		if err := ts.Expect('>'); err != nil {
			return nil, err
		}
		fieldType = "map"
	case firstType != "":
		fieldType = firstType
	default:
		var err *ParseError
		fieldType, err = ts.Identifier()
		if err != nil {
			return nil, err
		}
	}
	name, err := ts.Identifier()
	if err != nil {
		return nil, err
	}
	var fieldNumber int64
	if _, ok := ts.Consume("="); ok {
		text, err := ts.Number()
		if err != nil {
			return nil, err
		}
		if number, err := strconv.ParseFloat(text, 64); err == nil {
			fieldNumber = int64(number)
		}
	}
	if _, ok := ts.Consume("["); ok {
		skipProtoBalanced(ts, '[', ']')
	}
	if err := ts.Expect(';'); err != nil {
		return nil, err
	}
	field := Object()
	field.Set("name", StringNode(name))
	field.Set("type", StringNode(protoTypeFor(fieldType)))
	field.Set("require", BoolNode(label == "required"))
	field.Set("fieldNumber", IntNode(fieldNumber))
	if !protoIsScalar(fieldType) && fieldType != "map" {
		field.Set("typeName", StringNode(fieldType))
	}
	if fieldType == "bytes" {
		field.Set("format", StringNode("bytes"))
	}
	if label == "repeated" {
		field.Set("repeated", BoolNode(true))
		field.Set("childrenType", StringNode(protoTypeFor(fieldType)))
		field.Set("type", StringNode("array"))
	}
	if isMap {
		field.Set("type", StringNode("object"))
		field.Set("map", BoolNode(true))
		children := ArrayNode()
		for _, pair := range [][2]string{{"key", mapKey}, {"value", mapValue}} {
			child := Object()
			child.Set("name", StringNode(pair[0]))
			child.Set("type", StringNode(protoTypeFor(pair[1])))
			child.Set("typeName", StringNode(pair[1]))
			children.Push(child)
		}
		field.Set("children", children)
	}
	if inOneof {
		field.Set("oneof", StringNode(oneof))
	}
	return field, nil
}

func protoTypeFor(fieldType string) string {
	for _, entry := range protoScalars {
		if entry[0] == fieldType {
			return entry[1]
		}
	}
	return "object"
}

func protoIsScalar(fieldType string) bool {
	for _, entry := range protoScalars {
		if entry[0] == fieldType {
			return true
		}
	}
	return false
}

func parseProtoEnum(ts *TokenStream, symbols *Node) *ParseError {
	name, err := ts.Identifier()
	if err != nil {
		return err
	}
	values := ArrayNode()
	if err := ts.Expect('{'); err != nil {
		return err
	}
	for !ts.EOF() && ts.Peek(0) != "}" {
		if _, ok := ts.Consume(";"); ok {
			continue
		}
		value, err := ts.Identifier()
		if err != nil {
			return err
		}
		if _, ok := ts.Consume("="); ok {
			if _, err := ts.Number(); err != nil {
				return err
			}
			if _, ok := ts.Consume("["); ok {
				skipProtoBalanced(ts, '[', ']')
			}
		}
		ts.Consume(";")
		values.Push(StringNode(value))
	}
	if err := ts.Expect('}'); err != nil {
		return err
	}
	symbols.Get("enums").Set(name, values)
	return nil
}

func parseProtoService(ts *TokenStream, symbols *Node, ctx *Context, packageName string) *ParseError {
	name, err := ts.Identifier()
	if err != nil {
		return err
	}
	rpcs := ArrayNode()
	if err := ts.Expect('{'); err != nil {
		return err
	}
	for !ts.EOF() && ts.Peek(0) != "}" {
		line, column := ts.Line(), ts.Column()
		keyword, err := ts.Identifier()
		if err != nil {
			return err
		}
		if keyword == "option" {
			skipProtoToSemicolon(ts)
			continue
		}
		if keyword != "rpc" {
			ctx.Warning("unsupported service member: "+keyword, line, column)
			skipProtoDefinition(ts)
			continue
		}
		rpcName, err := ts.Identifier()
		if err != nil {
			return err
		}
		if err := ts.Expect('('); err != nil {
			return err
		}
		clientStream := false
		if ts.PeekIdentifier() == "stream" {
			if _, err := ts.Identifier(); err != nil {
				return err
			}
			clientStream = true
		}
		request, err := ts.Identifier()
		if err != nil {
			return err
		}
		if err := ts.Expect(')'); err != nil {
			return err
		}
		returns, err := ts.Identifier()
		if err != nil {
			return err
		}
		if returns != "returns" {
			ctx.Error("expected returns", line, column)
			skipProtoDefinition(ts)
			continue
		}
		if err := ts.Expect('('); err != nil {
			return err
		}
		serverStream := false
		if ts.PeekIdentifier() == "stream" {
			if _, err := ts.Identifier(); err != nil {
				return err
			}
			serverStream = true
		}
		response, err := ts.Identifier()
		if err != nil {
			return err
		}
		if err := ts.Expect(')'); err != nil {
			return err
		}
		if _, ok := ts.Consume("{"); ok {
			skipProtoBalanced(ts, '{', '}')
		}
		ts.Consume(";")
		rpc := Object()
		rpc.Set("name", StringNode(rpcName))
		rpc.Set("request", StringNode(request))
		rpc.Set("response", StringNode(response))
		rpc.Set("client_stream", BoolNode(clientStream))
		rpc.Set("server_stream", BoolNode(serverStream))
		rpcs.Push(rpc)
	}
	if err := ts.Expect('}'); err != nil {
		return err
	}
	service := Object()
	service.Set("name", StringNode(name))
	service.Set("package", StringNode(packageName))
	service.Set("rpcs", rpcs)
	symbols.Get("services").Set(name, service)
	return nil
}

func selectProtoRPC(rpcs []protoRPC, wanted string, ctx *Context) *protoRPC {
	if wanted != "" {
		for i := range rpcs {
			rpcName := rpcs[i].rpc.Get("name").ScalarString()
			if rpcs[i].operation == wanted || rpcs[i].service+"/"+rpcName == wanted {
				return &rpcs[i]
			}
		}
		ctx.Error("operation not found: "+wanted+"; candidates: "+protoCandidates(rpcs), 0, 0)
		return nil
	}
	if len(rpcs) == 1 {
		return &rpcs[0]
	}
	ctx.Error("operation required; candidates: "+protoCandidates(rpcs), 0, 0)
	return nil
}

func protoCandidates(rpcs []protoRPC) string {
	names := make([]string, len(rpcs))
	for i, rpc := range rpcs {
		names[i] = rpc.operation
	}
	return strings.Join(names, ", ")
}

func protoMessageForType(fieldType string, symbols *Node, ctx *Context, visited []string) []map[string]any {
	if message := protoFindMessage(fieldType, symbols); message != nil {
		for _, entry := range visited {
			if entry == fieldType {
				ctx.Warning("proto message cycle detected: "+fieldType, 0, 0)
				return []map[string]any{}
			}
		}
		nextVisited := append(append([]string{}, visited...), fieldType)
		fields := []map[string]any{}
		for _, entry := range entries(message.Get("fields")) {
			fields = append(fields, protoMapField(entry.Value, symbols, ctx, nextVisited))
		}
		return fields
	}
	if values := protoFindEnum(fieldType, symbols); values != nil {
		return []map[string]any{{
			"name": "value",
			"type": "string",
			"enum": values.ToAny(),
		}}
	}
	ctx.Warning("unresolved proto type: "+fieldType, 0, 0)
	return []map[string]any{}
}

func protoMapField(field, symbols *Node, ctx *Context, visited []string) map[string]any {
	fieldNumber := int64(0)
	if node := field.Get("fieldNumber"); node != nil {
		switch node.Kind {
		case intNode:
			fieldNumber = node.Int
		case floatNode:
			fieldNumber = int64(node.Num)
		}
	}
	result := map[string]any{
		"name":        field.Get("name").ScalarString(),
		"type":        field.Get("type").ScalarString(),
		"require":     field.Get("require").Truthy(),
		"fieldNumber": fieldNumber,
	}
	if field.Get("oneof").Truthy() {
		result["oneof"] = field.Get("oneof").ScalarString()
	}
	if field.Get("repeated").Truthy() {
		result["repeated"] = true
		result["type"] = "array"
	}
	if field.Get("map").Truthy() {
		result["map"] = true
		result["type"] = "object"
		result["children"] = field.Get("children").ToAny()
		return result
	}
	typeName := field.Get("typeName").ScalarString()
	repeated := field.Get("repeated").Truthy()
	if typeName != "" {
		if protoFindMessage(typeName, symbols) != nil {
			children := protoMessageForType(typeName, symbols, ctx, visited)
			if repeated {
				result["children"] = children
				result["childrenType"] = "object"
			} else {
				result["type"] = "object"
				result["ref"] = typeName
				result["children"] = children
			}
			return result
		}
		if values := protoFindEnum(typeName, symbols); values != nil {
			result["type"] = "string"
			result["enum"] = values.ToAny()
			return result
		}
		ctx.Warning("unresolved proto type: "+typeName, 0, 0)
		result["type"] = "object"
		result["ref"] = typeName
		return result
	}
	if repeated {
		result["childrenType"] = field.Get("childrenType").ScalarString()
		if result["childrenType"] == "" {
			result["childrenType"] = "string"
		}
	}
	return result
}

func protoFindMessage(fieldType string, symbols *Node) *Node {
	messages := symbols.Get("messages")
	if messages == nil {
		return nil
	}
	if message := messages.Get(fieldType); message != nil {
		return message
	}
	return messages.Get(protoLastSegment(fieldType))
}

func protoFindEnum(fieldType string, symbols *Node) *Node {
	enums := symbols.Get("enums")
	if enums == nil {
		return nil
	}
	if values := enums.Get(fieldType); values != nil {
		return values
	}
	return enums.Get(protoLastSegment(fieldType))
}

func protoLastSegment(fieldType string) string {
	if index := strings.LastIndexByte(fieldType, '.'); index >= 0 {
		return fieldType[index+1:]
	}
	return fieldType
}

func skipProtoToSemicolon(ts *TokenStream) {
	for !ts.EOF() {
		ch := ts.Peek(0)
		if ch == ";" {
			ts.NextChar()
			return
		}
		if ch == "{" {
			ts.NextChar()
			skipProtoBalanced(ts, '{', '}')
			continue
		}
		if ch == "[" {
			ts.NextChar()
			skipProtoBalanced(ts, '[', ']')
			continue
		}
		ts.NextChar()
	}
}

func skipProtoDefinition(ts *TokenStream) {
	for !ts.EOF() {
		ch := ts.Peek(0)
		if ch == "{" {
			ts.NextChar()
			skipProtoBalanced(ts, '{', '}')
			return
		}
		if ch == ";" {
			ts.NextChar()
			return
		}
		ts.NextChar()
	}
}

func skipProtoBalanced(ts *TokenStream, open, close rune) {
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

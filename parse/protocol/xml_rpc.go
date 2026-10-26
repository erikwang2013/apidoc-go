package protocol

import (
	"strconv"
	"strings"
)

var xmlRPCTypes = [][2]string{
	{"int", "integer"},
	{"i4", "integer"},
	{"i8", "integer"},
	{"double", "number"},
	{"boolean", "boolean"},
	{"string", "string"},
	{"dateTime.iso8601", "string"},
	{"base64", "string"},
	{"struct", "object"},
	{"array", "array"},
}

func parseXMLRPC(ctx *Context, content string) (Parsed, *ParseError) {
	document, err := LoadXML(content)
	if err != nil {
		return emptyParsed(), err
	}
	methods := Object()
	for _, method := range xmlDescendants(document.Root, "method") {
		params := ArrayNode()
		for _, param := range xmlDescendants(method, "param") {
			fieldType := param.Attr("type")
			field := Object()
			paramName := param.Attr("name")
			if paramName == "" {
				paramName = "param" + strconv.Itoa(len(params.Arr)+1)
			}
			field.Set("name", StringNode(paramName))
			field.Set("type", StringNode(xmlRPCType(fieldType)))
			field.Set("require", BoolNode(true))
			if fieldType == "struct" {
				children := ArrayNode()
				for _, member := range param.All("member") {
					child := Object()
					child.Set("name", StringNode(member.Attr("name")))
					child.Set("type", StringNode(xmlRPCType(member.Attr("type"))))
					children.Push(child)
				}
				field.Set("children", children)
			}
			params.Push(field)
		}
		resultType := "string"
		if result := method.First("result"); result != nil {
			resultType = xmlRPCType(result.Attr("type"))
		}
		entry := Object()
		entry.Set("params", params)
		entry.Set("result", StringNode(resultType))
		methods.Set(method.Attr("name"), entry)
	}
	names := methods.Keys()
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
	if method == nil {
		ctx.Error("operation not found: "+wanted+"; candidates: "+strings.Join(names, ", "), 0, 0)
		return emptyParsed(), nil
	}
	params := method.Get("params")
	if params == nil {
		params = ArrayNode()
	}
	resultType := method.Get("result").ScalarString()
	if resultType == "" {
		resultType = "string"
	}
	resultField := Object()
	resultField.Set("name", StringNode("result"))
	resultField.Set("type", StringNode(resultType))
	resultField.Set("require", BoolNode(true))
	paramFields := make([]map[string]any, 0, len(params.Arr))
	for _, param := range params.Arr {
		paramFields = append(paramFields, param.ToAny().(map[string]any))
	}
	messages := []map[string]any{
		newMessage("Params", "request", paramFields),
		newMessage("Result", "response", []map[string]any{resultField.ToAny().(map[string]any)}),
	}
	if sample := ctx.Options().Sample; sample != "" {
		messages[0]["sampleValidation"] = validateXMLRPCSample(sample, wanted, params)
	}
	return Parsed{Operation: wanted, Messages: messages, Meta: map[string]any{}}, nil
}

func xmlDescendants(node *XMLNode, local string) []*XMLNode {
	var result []*XMLNode
	for _, child := range node.Children {
		if child.Name == local {
			result = append(result, child)
		}
		result = append(result, xmlDescendants(child, local)...)
	}
	return result
}

func xmlRPCType(fieldType string) string {
	for _, entry := range xmlRPCTypes {
		if entry[0] == fieldType {
			return entry[1]
		}
	}
	return "string"
}

func validateXMLRPCSample(sample, method string, params *Node) SampleValidation {
	errorValidation := func(message string) SampleValidation {
		return SampleValidation{
			OK: false,
			Errors: []SampleError{{
				Path: "$", Severity: "error", Message: message,
			}},
			Warnings: []SampleError{},
		}
	}
	document, err := LoadXML(sample)
	if err != nil {
		return errorValidation(err.Message)
	}
	root := document.Root
	if root.Name != "methodCall" && root.Name != "methodResponse" {
		return errorValidation("root must be methodCall or methodResponse")
	}
	if root.Name == "methodResponse" {
		if root.First("params") == nil && root.First("fault") == nil {
			return errorValidation("methodResponse requires params or fault")
		}
		return okSampleValidation()
	}
	methodName := root.First("methodName")
	if methodName == nil || strings.TrimFunc(methodName.TextContent(), isPHPTrimRune) != method {
		return errorValidation("methodName mismatch")
	}
	actual := 0
	if paramsNode := root.First("params"); paramsNode != nil {
		actual = len(paramsNode.All("param"))
	}
	expected := len(params.Arr)
	if actual != expected {
		return errorValidation("param count mismatch")
	}
	return okSampleValidation()
}

// isPHPTrimRune is PHP trim()'s default character set.
func isPHPTrimRune(ch rune) bool {
	switch ch {
	case ' ', '\t', '\n', '\r', '\x00', '\v':
		return true
	}
	return false
}

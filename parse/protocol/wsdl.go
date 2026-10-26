package protocol

import (
	"strconv"
	"strings"
)

var xsdScalars = [][2]string{
	{"string", "string"}, {"normalizedString", "string"}, {"token", "string"},
	{"int", "integer"}, {"integer", "integer"}, {"long", "integer"}, {"short", "integer"},
	{"byte", "integer"}, {"unsignedInt", "integer"}, {"unsignedLong", "integer"},
	{"unsignedShort", "integer"}, {"decimal", "number"}, {"float", "number"},
	{"double", "number"}, {"boolean", "boolean"}, {"dateTime", "string"},
	{"date", "string"}, {"time", "string"}, {"base64Binary", "string"}, {"anyType", "string"},
}

type wsdlOperation struct {
	name, input, output string
}

type wsdlCandidate struct {
	label, service, port, portType string
	operation                      wsdlOperation
}

func parseWSDL(ctx *Context, content string) (Parsed, *ParseError) {
	document, err := LoadXML(content)
	if err != nil {
		return emptyParsed(), err
	}
	if document.Root.Name != "definitions" {
		ctx.Error("WSDL definitions element is missing", 0, 0)
		return emptyParsed(), nil
	}
	definitions := document.Root
	elements := Object()
	complexTypes := Object()
	simpleTypes := Object()
	messages := Object()
	portTypes := Object()
	bindings := Object()
	services := Object()

	if types := definitions.First("types"); types != nil {
		for _, schemaNode := range types.All("schema") {
			if err := parseWSDLSchema(schemaNode, ctx, elements, complexTypes, simpleTypes); err != nil {
				return emptyParsed(), err
			}
		}
	}
	for _, messageNode := range definitions.All("message") {
		parts := ArrayNode()
		for _, part := range messageNode.All("part") {
			entry := Object()
			entry.Set("name", StringNode(part.Attr("name")))
			entry.Set("element", StringNode(LocalName(part.Attr("element"))))
			entry.Set("type", StringNode(LocalName(part.Attr("type"))))
			parts.Push(entry)
		}
		messages.Set(messageNode.Attr("name"), parts)
	}
	for _, portType := range definitions.All("portType") {
		operations := ArrayNode()
		for _, operation := range portType.All("operation") {
			entry := Object()
			entry.Set("name", StringNode(operation.Attr("name")))
			input, output := "", ""
			if node := operation.First("input"); node != nil {
				input = LocalName(node.Attr("message"))
			}
			if node := operation.First("output"); node != nil {
				output = LocalName(node.Attr("message"))
			}
			entry.Set("input", StringNode(input))
			entry.Set("output", StringNode(output))
			operations.Push(entry)
		}
		portTypes.Set(portType.Attr("name"), operations)
	}
	for _, binding := range definitions.All("binding") {
		bindings.Set(binding.Attr("name"), StringNode(LocalName(binding.Attr("type"))))
	}
	for _, service := range definitions.All("service") {
		ports := ArrayNode()
		for _, port := range service.All("port") {
			entry := Object()
			entry.Set("name", StringNode(port.Attr("name")))
			entry.Set("binding", StringNode(LocalName(port.Attr("binding"))))
			ports.Push(entry)
		}
		services.Set(service.Attr("name"), ports)
	}

	candidates := wsdlCandidates(services, bindings, portTypes)
	selected := selectWSDL(candidates, ctx.Options().Operation, ctx)
	if selected == nil {
		return emptyParsed(), nil
	}
	requestFields := wsdlMessageFields(selected.operation.input, messages, elements, complexTypes, simpleTypes)
	responseFields := wsdlMessageFields(selected.operation.output, messages, elements, complexTypes, simpleTypes)
	requestName := selected.operation.input
	if requestName == "" {
		requestName = "Request"
	}
	responseName := selected.operation.output
	if responseName == "" {
		responseName = "Response"
	}
	parsedMessages := []map[string]any{
		newMessage(requestName, "request", requestFields),
		newMessage(responseName, "response", responseFields),
	}
	if sample := ctx.Options().Sample; sample != "" {
		parsedMessages[0]["sampleValidation"] = validateWSDLSample(sample, selected.operation)
	}
	return Parsed{
		Operation: selected.label,
		Messages:  parsedMessages,
		Meta:      map[string]any{"service": selected.service, "port": selected.port},
	}, nil
}

func parseWSDLSchema(schemaNode *XMLNode, ctx *Context, elements, complexTypes, simpleTypes *Node) *ParseError {
	for _, tag := range []string{"include", "import"} {
		for _, include := range schemaNode.All(tag) {
			location := include.Attr("schemaLocation")
			if location == "" {
				continue
			}
			realpath, ok := ctx.Resolve(location)
			if !ok {
				ctx.Warning("unable to resolve XSD "+tag+": "+location, 0, 0)
				continue
			}
			if !ctx.Push(realpath) {
				continue
			}
			content, read := ctx.Read()
			if read {
				included, err := LoadXML(content)
				if err != nil {
					ctx.Pop()
					return err
				}
				if err := parseWSDLSchema(included.Root, ctx, elements, complexTypes, simpleTypes); err != nil {
					ctx.Pop()
					return err
				}
			}
			ctx.Pop()
		}
	}
	for _, element := range schemaNode.All("element") {
		name := element.Attr("name")
		if name == "" {
			continue
		}
		fieldType := LocalName(element.Attr("type"))
		entry := Object()
		entry.Set("name", StringNode(name))
		entry.Set("type", StringNode(fieldType))
		fields := ArrayNode()
		if inline := element.First("complexType"); inline != nil {
			fields = wsdlComplexType(inline)
		}
		entry.Set("fields", fields)
		values := ArrayNode()
		if simple := element.First("simpleType"); simple != nil {
			values = wsdlSimpleType(simple)
		}
		entry.Set("enum", values)
		elements.Set(name, entry)
	}
	for _, complexType := range schemaNode.All("complexType") {
		if name := complexType.Attr("name"); name != "" {
			complexTypes.Set(name, wsdlComplexType(complexType))
		}
	}
	for _, simpleType := range schemaNode.All("simpleType") {
		if name := simpleType.Attr("name"); name != "" {
			simpleTypes.Set(name, wsdlSimpleType(simpleType))
		}
	}
	return nil
}

func wsdlComplexType(complexType *XMLNode) *Node {
	fields := ArrayNode()
	container := complexType.First("sequence")
	if container == nil {
		container = complexType.First("all")
	}
	if container != nil {
		for _, element := range container.All("element") {
			fields.Push(wsdlXSDElement(element))
		}
	}
	for _, attribute := range complexType.All("attribute") {
		name := attribute.Attr("name")
		if name == "" {
			continue
		}
		field := Object()
		field.Set("name", StringNode(name))
		field.Set("type", StringNode(wsdlXSDType(LocalName(attribute.Attr("type")))))
		field.Set("require", BoolNode(attribute.Attr("use") == "required"))
		fields.Push(field)
	}
	return fields
}

func wsdlXSDElement(element *XMLNode) *Node {
	name := element.Attr("name")
	fieldType := LocalName(element.Attr("type"))
	inline := element.First("complexType")
	simple := element.First("simpleType")
	min := element.Attr("minOccurs")
	max := element.Attr("maxOccurs")
	field := Object()
	field.Set("name", StringNode(name))
	if inline != nil {
		field.Set("type", StringNode("object"))
	} else {
		field.Set("type", StringNode(wsdlXSDType(fieldType)))
	}
	field.Set("require", BoolNode(min != "0"))
	if inline != nil {
		field.Set("children", wsdlComplexType(inline))
	} else if fieldType != "" {
		field.Set("typeName", StringNode(fieldType))
	}
	if simple != nil {
		field.Set("enum", wsdlSimpleType(simple))
		field.Set("type", StringNode("string"))
	}
	if wsdlIsRepeated(max) {
		current := field.Get("type").ScalarString()
		if current == "" {
			current = "string"
		}
		field.Set("repeated", BoolNode(true))
		field.Set("childrenType", StringNode(current))
		field.Set("type", StringNode("array"))
	}
	return field
}

func wsdlIsRepeated(max string) bool {
	if max == "unbounded" {
		return true
	}
	if max == "" {
		return false
	}
	for i := 0; i < len(max); i++ {
		if max[i] < '0' || max[i] > '9' {
			return false
		}
	}
	number, _ := strconv.ParseInt(max, 10, 64)
	return number > 1
}

func wsdlSimpleType(simpleType *XMLNode) *Node {
	values := ArrayNode()
	for _, enumeration := range simpleType.All("enumeration") {
		values.Push(StringNode(enumeration.Attr("value")))
	}
	return values
}

func wsdlMessageFields(messageName string, messages, elements, complexTypes, simpleTypes *Node) []map[string]any {
	if messageName == "" {
		return nil
	}
	parts, ok := messages.Get(messageName).AsArray()
	if !ok {
		return nil
	}
	fields := []map[string]any{}
	for _, part := range parts {
		elementName := part.Get("element").ScalarString()
		typeName := part.Get("type").ScalarString()
		if elementName != "" {
			if element := elements.Get(elementName); element != nil {
				elementFields := element.Get("fields")
				if elementFields == nil {
					elementFields = ArrayNode()
				}
				for _, field := range wsdlResolveFields(elementFields, elements, complexTypes, simpleTypes, nil) {
					fields = append(fields, field.ToAny().(map[string]any))
				}
				continue
			}
		}
		if typeName != "" {
			name := part.Get("name").ScalarString()
			if name == "" {
				name = "value"
			}
			fields = append(fields, map[string]any{"name": name, "type": wsdlXSDType(typeName)})
		}
	}
	return fields
}

func wsdlResolveFields(fields, elements, complexTypes, simpleTypes *Node, visited []string) []*Node {
	result := []*Node{}
	for _, entry := range entries(fields) {
		field := entry.Value
		typeName := field.Get("typeName").ScalarString()
		if typeName != "" && complexTypes.Get(typeName) != nil && !containsString(visited, typeName) {
			nextVisited := append(append([]string{}, visited...), typeName)
			field.Set("type", StringNode("object"))
			children := wsdlResolveFields(complexTypes.Get(typeName), elements, complexTypes, simpleTypes, nextVisited)
			field.Set("children", arrayFromNodes(children))
		} else if typeName != "" {
			if element := elements.Get(typeName); element != nil {
				field.Set("type", StringNode("object"))
				children := wsdlResolveFields(element.Get("fields"), elements, complexTypes, simpleTypes, visited)
				field.Set("children", arrayFromNodes(children))
			}
		}
		if field.Get("children").Truthy() {
			children := field.Get("children")
			if children == nil {
				children = ArrayNode()
			}
			field.Set("children", arrayFromNodes(wsdlResolveFields(children, elements, complexTypes, simpleTypes, visited)))
		}
		result = append(result, field)
	}
	return result
}

// arrayFromNodes wraps existing nodes in an array node.
func arrayFromNodes(values []*Node) *Node {
	return &Node{Kind: arrayNode, Arr: values}
}

func wsdlCandidates(services, bindings, portTypes *Node) []wsdlCandidate {
	candidates := []wsdlCandidate{}
	for _, serviceEntry := range entries(services) {
		for _, port := range entries(serviceEntry.Value) {
			binding := port.Value.Get("binding").ScalarString()
			portType := bindings.Get(binding).ScalarString()
			operations, _ := portTypes.Get(portType).AsArray()
			for _, operationNode := range operations {
				operation := wsdlOperationFrom(operationNode)
				candidates = append(candidates, wsdlCandidate{
					label:     serviceEntry.Key + "/" + port.Value.Get("name").ScalarString() + "/" + operation.name,
					service:   serviceEntry.Key,
					port:      port.Value.Get("name").ScalarString(),
					portType:  portType,
					operation: operation,
				})
			}
		}
	}
	if len(candidates) == 0 {
		for _, portTypeEntry := range entries(portTypes) {
			for _, operationNode := range entries(portTypeEntry.Value) {
				operation := wsdlOperationFrom(operationNode.Value)
				candidates = append(candidates, wsdlCandidate{
					label:     portTypeEntry.Key + "/" + operation.name,
					portType:  portTypeEntry.Key,
					operation: operation,
				})
			}
		}
	}
	return candidates
}

func wsdlOperationFrom(operation *Node) wsdlOperation {
	return wsdlOperation{
		name:   operation.Get("name").ScalarString(),
		input:  operation.Get("input").ScalarString(),
		output: operation.Get("output").ScalarString(),
	}
}

func selectWSDL(candidates []wsdlCandidate, wanted string, ctx *Context) *wsdlCandidate {
	if wanted != "" {
		for i := range candidates {
			if candidates[i].label == wanted ||
				candidates[i].portType+"/"+candidates[i].operation.name == wanted ||
				candidates[i].operation.name == wanted {
				return &candidates[i]
			}
		}
		ctx.Error("operation not found: "+wanted+"; candidates: "+wsdlCandidateLabels(candidates), 0, 0)
		return nil
	}
	if len(candidates) == 1 {
		return &candidates[0]
	}
	ctx.Error("operation required; candidates: "+wsdlCandidateLabels(candidates), 0, 0)
	return nil
}

func wsdlCandidateLabels(candidates []wsdlCandidate) string {
	labels := make([]string, len(candidates))
	for i, candidate := range candidates {
		labels[i] = candidate.label
	}
	return strings.Join(labels, ", ")
}

func wsdlXSDType(fieldType string) string {
	if fieldType == "" {
		return "string"
	}
	for _, entry := range xsdScalars {
		if entry[0] == fieldType {
			return entry[1]
		}
	}
	return "object"
}

func validateWSDLSample(sample string, operation wsdlOperation) SampleValidation {
	errorValidation := func(message string) SampleValidation {
		return SampleValidation{
			OK:       false,
			Errors:   []SampleError{{Path: "$", Severity: "error", Message: message}},
			Warnings: []SampleError{},
		}
	}
	document, err := LoadXML(sample)
	if err != nil {
		return errorValidation(err.Message)
	}
	errors := []SampleError{}
	if document.Root.Name == "Envelope" {
		body := document.Root.First("Body")
		var operationElement *XMLNode
		if body != nil {
			operationElement = body.FirstElement()
		}
		if operationElement != nil {
			if operation.input != "" && operationElement.Name != operation.input {
				errors = append(errors, SampleError{
					Path: "$", Severity: "error", Message: "SOAP Body operation mismatch",
				})
			}
		} else {
			errors = append(errors, SampleError{
				Path: "$", Severity: "error", Message: "SOAP Body is empty",
			})
		}
	}
	return SampleValidation{OK: len(errors) == 0, Errors: errors, Warnings: []SampleError{}}
}

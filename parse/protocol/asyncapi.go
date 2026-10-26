package protocol

import (
	"strings"
)

type asyncCandidate struct {
	channel   string
	action    string
	operation *Node
	root      *Node
	file      string
}

type asyncSelection struct {
	operation string
	messages  []*Node
	roots     []*Node
	files     []string
}

func parseAsyncAPI(ctx *Context, content string) (Parsed, *ParseError) {
	document, err := decodeAsyncAPI(content)
	if err != nil {
		return emptyParsed(), err
	}
	version := ""
	if node := document.Get("asyncapi"); node != nil && node.Kind != nullNode {
		version = node.ScalarString()
	}
	if version == "" {
		ctx.Error("AsyncAPI version is missing", 0, 0)
	} else if !strings.HasPrefix(version, "2.") && !strings.HasPrefix(version, "3.") {
		ctx.Unsupported("unsupported AsyncAPI version: "+version, 0, 0)
	}
	wanted := ctx.Options().Operation
	var selection *asyncSelection
	if strings.HasPrefix(version, "3.") {
		selection = selectAsyncV3(document, wanted, ctx)
	} else {
		selection = selectAsyncV2(document, wanted, ctx)
	}
	if selection == nil {
		return emptyParsed(), nil
	}
	direction := ctx.Options().Direction
	if direction == "" {
		direction = "event"
	}
	sample := ctx.Options().Sample
	refs := refsFor(document, LoaderJSONOrYAML)
	messages := []map[string]any{}
	for index, messageNode := range selection.messages {
		if !messageNode.IsArrayLike() {
			continue
		}
		messageRoot := document
		if index < len(selection.roots) {
			messageRoot = selection.roots[index]
		}
		messageFile := ctx.File()
		if index < len(selection.files) {
			messageFile = selection.files[index]
		}
		payload := messageNode.Get("payload")
		if payload == nil || !payload.IsArrayLike() {
			payload = Object()
		}
		payload = expandSchema(payload, messageRoot, ctx, LoaderJSONOrYAML, messageFile)
		fields := SchemaFields(payload, ctx, refs, 0)
		name := ""
		if node := messageNode.Get("name"); node != nil && node.Kind != nullNode {
			name = node.ScalarString()
		}
		if name == "" {
			if node := payload.Get("title"); node != nil && node.Kind != nullNode {
				name = node.ScalarString()
			}
		}
		if name == "" {
			name = "Message"
		}
		message := newMessage(name, direction, fields)
		if sample != "" {
			message["sampleValidation"] = sampleValidationFor(sample, payload, nil, refs, ctx)
		}
		messages = append(messages, message)
	}
	return Parsed{Operation: selection.operation, Messages: messages, Meta: map[string]any{}}, nil
}

func decodeAsyncAPI(content string) (*Node, *ParseError) {
	trimmed := trimDocStart(content)
	if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
		node, message := ParseJSON(content)
		if message != "" {
			return nil, &ParseError{Message: "AsyncAPI JSON parse error: " + message}
		}
		return node, nil
	}
	return parseYAML(content)
}

func selectAsyncV2(document *Node, wanted string, ctx *Context) *asyncSelection {
	channels := document.Get("channels")
	if channels == nil || !channels.IsArrayLike() {
		channels = Object()
	}
	var candidates []asyncCandidate
	for _, channelEntry := range entries(channels) {
		channel := channelEntry.Value
		if !channel.IsArrayLike() {
			continue
		}
		root := document
		file := ctx.File()
		if refstr, ok := channel.Get("$ref").AsStr(); ok {
			if resolved, ok := resolveRef(refstr, document, ctx, LoaderJSONOrYAML); ok {
				channel = resolved.Node
				root = resolved.Root
				file = resolved.File
			}
		}
		for _, action := range []string{"publish", "subscribe"} {
			if operation := channel.Get(action); operation != nil && operation.IsArrayLike() {
				candidates = append(candidates, asyncCandidate{
					channel: channelEntry.Key, action: action, operation: operation, root: root, file: file,
				})
			}
		}
	}
	names := func() string {
		parts := make([]string, len(candidates))
		for i, candidate := range candidates {
			parts[i] = candidate.channel + ":" + candidate.action
		}
		return strings.Join(parts, ", ")
	}
	if wanted != "" {
		channelWanted, actionWanted := wanted, ""
		if index := strings.LastIndexByte(wanted, ':'); index >= 0 {
			channelWanted, actionWanted = wanted[:index], wanted[index+1:]
		}
		for i := range candidates {
			if candidates[i].channel == channelWanted && (actionWanted == "" || candidates[i].action == actionWanted) {
				return asyncV2Messages(&candidates[i], document, ctx)
			}
		}
		ctx.Error("operation not found: "+wanted+"; candidates: "+names(), 0, 0)
		return nil
	}
	if len(candidates) == 1 {
		return asyncV2Messages(&candidates[0], document, ctx)
	}
	ctx.Error("operation required; candidates: "+names(), 0, 0)
	return nil
}

func asyncV2Messages(candidate *asyncCandidate, document *Node, ctx *Context) *asyncSelection {
	operation := candidate.channel + ":" + candidate.action
	message := candidate.operation.Get("message")
	if message == nil {
		message = Object()
	}
	messageRoot := candidate.root
	messageFile := candidate.file
	if refstr, ok := message.Get("$ref").AsStr(); ok {
		if resolved, ok := resolveRef(refstr, document, ctx, LoaderJSONOrYAML); ok {
			message = resolved.Node
			messageRoot = resolved.Root
			messageFile = resolved.File
		} else {
			message = nil
		}
	}
	messages := []*Node{}
	if message != nil && message.IsArrayLike() {
		messages = append(messages, message)
	}
	return &asyncSelection{
		operation: operation,
		messages:  messages,
		roots:     []*Node{messageRoot},
		files:     []string{messageFile},
	}
}

func selectAsyncV3(document *Node, wanted string, ctx *Context) *asyncSelection {
	operations := document.Get("operations")
	if operations == nil || !operations.IsArrayLike() {
		operations = Object()
	}
	type candidate struct {
		id        string
		operation *Node
	}
	var candidates []candidate
	for _, entry := range entries(operations) {
		if entry.Value.IsArrayLike() {
			candidates = append(candidates, candidate{id: entry.Key, operation: entry.Value})
		}
	}
	names := make([]string, len(candidates))
	for i, candidate := range candidates {
		names[i] = candidate.id
	}
	joined := strings.Join(names, ", ")
	if wanted != "" {
		for _, candidate := range candidates {
			if candidate.id == wanted {
				return asyncV3Messages(candidate.id, candidate.operation, document, ctx)
			}
		}
		ctx.Error("operation not found: "+wanted+"; candidates: "+joined, 0, 0)
		return nil
	}
	if len(candidates) == 1 {
		return asyncV3Messages(candidates[0].id, candidates[0].operation, document, ctx)
	}
	ctx.Error("operation required; candidates: "+joined, 0, 0)
	return nil
}

func asyncV3Messages(id string, operation, document *Node, ctx *Context) *asyncSelection {
	// The channel ref is resolved for its diagnostic side effects only.
	if refstr, ok := operation.Get("channel").Get("$ref").AsStr(); ok {
		_, _ = resolveRef(refstr, document, ctx, LoaderJSONOrYAML)
	}
	selection := &asyncSelection{operation: id}
	messageRefs := operation.Get("messages")
	if messageRefs == nil || !messageRefs.IsArrayLike() {
		return selection
	}
	for _, entry := range entries(messageRefs) {
		messageRef := entry.Value
		messageRoot := document
		messageFile := ctx.File()
		if refstr, ok := messageRef.Get("$ref").AsStr(); ok {
			if resolved, ok := resolveRef(refstr, document, ctx, LoaderJSONOrYAML); ok {
				messageRef = resolved.Node
				messageRoot = resolved.Root
				messageFile = resolved.File
			} else {
				messageRef = nil
			}
		}
		if messageRef != nil && messageRef.IsArrayLike() {
			selection.messages = append(selection.messages, messageRef)
			selection.roots = append(selection.roots, messageRoot)
			selection.files = append(selection.files, messageFile)
		}
	}
	return selection
}

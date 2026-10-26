package protocol

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

const (
	maxRefs  = 64
	maxDepth = 32
)

type Resolved struct {
	Node *Node
	Root *Node
	File string
}

// RefCtx bundles the document root a set of schema helpers resolves against.
type RefCtx struct {
	Root   *Node
	File   string
	Loader Loader
}

func NewRefCtx(root *Node, file string, loader Loader) RefCtx {
	return RefCtx{Root: root, File: file, Loader: loader}
}

func (r *RefCtx) Resolve(refstr string, ctx *Context) *Node {
	entered := r.File != "" && r.File != ctx.File()
	if entered {
		ctx.EnterFile(r.File)
	}
	resolved, ok := resolveRef(refstr, r.Root, ctx, r.Loader)
	if entered {
		ctx.LeaveFile()
	}
	if !ok {
		return nil
	}
	return resolved.Node
}

func resolveRef(refstr string, root *Node, ctx *Context, loader Loader) (*Resolved, bool) {
	file, fragment, ok := splitRef(refstr, ctx)
	if !ok {
		return nil, false
	}
	resolvedRoot := root
	resolvedFile := ctx.File()
	if file != "" {
		errorsBefore := len(ctx.Errors())
		realpath, ok := ctx.Resolve(file)
		if !ok {
			refError(ctx, errorsBefore, refstr)
			return nil, false
		}
		resolvedFile = realpath
		if document := ctx.Document(realpath); document != nil {
			resolvedRoot = document
		} else {
			if !ctx.Push(realpath) {
				return nil, false
			}
			content, read := ctx.Read()
			var document *Node
			if read {
				document, _ = loader.Load(content)
			}
			ctx.Pop()
			if document == nil {
				ctx.Error("unable to parse $ref: "+refstr, 0, 0)
				return nil, false
			}
			ctx.RememberDocument(realpath, document)
			resolvedRoot = document
		}
	}
	node, ok := pointer(resolvedRoot, fragment, ctx, refstr)
	if !ok {
		return nil, false
	}
	return &Resolved{Node: node, Root: resolvedRoot, File: resolvedFile}, true
}

func expandSchema(schema, root *Node, ctx *Context, loader Loader, baseFile string) *Node {
	entered := baseFile != "" && baseFile != ctx.File()
	if entered {
		ctx.EnterFile(baseFile)
	}
	state := &expandState{}
	result := expandNode(schema, root, ctx, loader, state, 0)
	if entered {
		ctx.LeaveFile()
	}
	return result
}

type expandState struct {
	refs  int
	stack []string
}

func expandNode(node, root *Node, ctx *Context, loader Loader, state *expandState, depth int) *Node {
	if depth > maxDepth || state.refs > maxRefs {
		return stub(ctx, "$ref depth limit exceeded")
	}
	if refstr, ok := node.Get("$ref").AsStr(); ok {
		siblings := node.Clone()
		siblings.Remove("$ref")
		siblings = expandNode(siblings, root, ctx, loader, state, depth)
		return expandRef(refstr, siblings, root, ctx, loader, state, depth)
	}
	node = node.Clone()
	for _, key := range []string{"properties", "$defs", "definitions"} {
		if value := node.Get(key); value != nil {
			node.Set(key, expandChildren(value, root, ctx, loader, state, depth))
		}
	}
	for _, key := range []string{"allOf", "anyOf", "oneOf"} {
		if value := node.Get(key); value != nil {
			node.Set(key, expandChildren(value, root, ctx, loader, state, depth))
		}
	}
	if items := node.Get("items"); items != nil {
		if items.IsList() {
			node.Set("items", expandChildren(items, root, ctx, loader, state, depth))
		} else {
			node.Set("items", expandNode(items, root, ctx, loader, state, depth))
		}
	}
	return node
}

func expandChildren(value, root *Node, ctx *Context, loader Loader, state *expandState, depth int) *Node {
	switch value.Kind {
	case arrayNode:
		out := ArrayNode()
		for _, child := range value.Arr {
			out.Push(expandNode(child, root, ctx, loader, state, depth))
		}
		return out
	case objectNode:
		out := Object()
		for _, member := range value.Obj {
			out.Set(member.Key, expandNode(member.Value, root, ctx, loader, state, depth))
		}
		return out
	}
	return value.Clone()
}

func expandRef(refstr string, siblings, root *Node, ctx *Context, loader Loader, state *expandState, depth int) *Node {
	file, fragment, ok := splitRef(refstr, ctx)
	if !ok {
		return Merge(siblings, emptyStub())
	}
	realpath := ""
	key := ctx.File() + "#" + fragment
	if file != "" {
		errorsBefore := len(ctx.Errors())
		resolved, ok := ctx.Resolve(file)
		if !ok {
			refError(ctx, errorsBefore, refstr)
			return Merge(siblings, emptyStub())
		}
		realpath = resolved
		key = realpath + "#" + fragment
	}
	for _, entry := range state.stack {
		if entry == key {
			return Merge(siblings, stub(ctx, "$ref cycle detected: "+refstr))
		}
	}
	if state.refs >= maxRefs || depth >= maxDepth {
		return Merge(siblings, stub(ctx, "$ref depth limit exceeded: "+refstr))
	}
	state.refs++
	state.stack = append(state.stack, key)
	var documentRoot *Node
	if realpath != "" {
		if document := ctx.ExpandedDocument(realpath); document != nil {
			documentRoot = document
		} else {
			if !ctx.Push(realpath) {
				state.stack = state.stack[:len(state.stack)-1]
				return Merge(siblings, emptyStub())
			}
			content, read := ctx.Read()
			var document *Node
			if read {
				document, _ = loader.Load(content)
			}
			var expanded *Node
			if document != nil {
				expanded = expandNode(document, document, ctx, loader, state, depth+1)
			} else {
				expanded = ArrayNode()
			}
			ctx.Pop()
			if document == nil {
				state.stack = state.stack[:len(state.stack)-1]
				ctx.Error("unable to parse $ref: "+refstr, 0, 0)
				return Merge(siblings, emptyStub())
			}
			ctx.RememberExpanded(realpath, expanded)
			documentRoot = expanded
		}
	} else {
		documentRoot = root
	}
	target, ok := pointer(documentRoot, fragment, ctx, refstr)
	if !ok {
		state.stack = state.stack[:len(state.stack)-1]
		return Merge(siblings, emptyStub())
	}
	expanded := expandNode(target, documentRoot, ctx, loader, state, depth+1)
	state.stack = state.stack[:len(state.stack)-1]
	return Merge(siblings, expanded)
}

// refError: a read failure was already recorded by Context.Resolve, which PHP
// reports as "unable to parse", not "unable to resolve".
func refError(ctx *Context, errorsBefore int, refstr string) {
	action := "resolve"
	if len(ctx.Errors()) > errorsBefore {
		action = "parse"
	}
	ctx.Error("unable to "+action+" $ref: "+refstr, 0, 0)
}

func splitRef(refstr string, ctx *Context) (string, string, bool) {
	refstr = strings.TrimSpace(refstr)
	if refstr == "" {
		ctx.Error("empty $ref", 0, 0)
		return "", "", false
	}
	if strings.HasPrefix(refstr, "#") {
		return "", refstr[1:], true
	}
	if index := strings.IndexByte(refstr, '#'); index >= 0 {
		return refstr[:index], refstr[index+1:], true
	}
	return refstr, "", true
}

func pointer(root *Node, fragment string, ctx *Context, refstr string) (*Node, bool) {
	if fragment == "" || fragment == "/" {
		return root, true
	}
	node := root
	for _, segment := range strings.Split(strings.TrimPrefix(fragment, "/"), "/") {
		segment = rawurldecode(strings.ReplaceAll(strings.ReplaceAll(segment, "~1", "/"), "~0", "~"))
		var child *Node
		switch node.Kind {
		case objectNode:
			child = node.Get(segment)
		case arrayNode:
			if index, err := strconv.Atoi(segment); err == nil && index >= 0 && index < len(node.Arr) {
				child = node.Arr[index]
			}
		}
		if child == nil {
			ctx.Error("$ref not found: "+refstr, 0, 0)
			return nil, false
		}
		node = child
	}
	if !node.IsArrayLike() {
		ctx.Error("$ref target is not an object: "+refstr, 0, 0)
		return nil, false
	}
	return node, true
}

func stub(ctx *Context, message string) *Node {
	ctx.Warning(message, 0, 0)
	return emptyStub()
}

func emptyStub() *Node {
	node := Object()
	node.Set("type", StringNode("object"))
	node.Set("properties", Object())
	node.Set("children", ArrayNode())
	return node
}

func rawurldecode(value string) string {
	bytes := []byte(value)
	var out []byte
	for index := 0; index < len(bytes); index++ {
		if bytes[index] == '%' && index+2 < len(bytes) && isHexByte(bytes[index+1]) && isHexByte(bytes[index+2]) {
			if decoded, err := strconv.ParseUint(string(bytes[index+1:index+3]), 16, 8); err == nil {
				out = append(out, byte(decoded))
				index += 2
				continue
			}
		}
		out = append(out, bytes[index])
	}
	return string(out)
}

func isHexByte(ch byte) bool {
	return (ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'f') || (ch >= 'A' && ch <= 'F')
}

// Entry is one key/value of a PHP-style array.
type Entry struct {
	Key   string
	Value *Node
}

func entries(value *Node) []Entry {
	if value == nil {
		return nil
	}
	switch value.Kind {
	case objectNode:
		result := make([]Entry, len(value.Obj))
		for i, member := range value.Obj {
			result[i] = Entry{Key: member.Key, Value: member.Value}
		}
		return result
	case arrayNode:
		result := make([]Entry, len(value.Arr))
		for i, item := range value.Arr {
			result[i] = Entry{Key: strconv.Itoa(i), Value: item}
		}
		return result
	}
	return nil
}

func nodeValues(value *Node) []any {
	items := entries(value)
	result := make([]any, len(items))
	for i, item := range items {
		result[i] = item.Value.ToAny()
	}
	return result
}

// SchemaFields maps a JSON Schema object to UIC message fields.
func SchemaFields(schema *Node, ctx *Context, refs *RefCtx, depth int) []map[string]any {
	if depth > 32 {
		ctx.Warning("schema nesting limit reached", 0, 0)
		return []map[string]any{}
	}
	schema = flattenSchema(schema, ctx, refs, map[string]bool{}, 0)
	properties := schema.Get("properties")
	if properties == nil || !properties.IsArrayLike() {
		return []map[string]any{}
	}
	propEntries := entries(properties)
	if len(propEntries) == 0 {
		return []map[string]any{}
	}
	var required []string
	if list, ok := schema.Get("required").AsArray(); ok {
		for _, item := range list {
			if name, ok := item.AsStr(); ok {
				required = append(required, name)
			}
		}
	}
	result := []map[string]any{}
	for _, propEntry := range propEntries {
		name := propEntry.Key
		prop := propEntry.Value
		if prop.IsArrayLike() {
			prop = flattenSchema(prop, ctx, refs, map[string]bool{}, 0)
		} else {
			prop = Object()
		}
		fieldType := SchemaTypeOf(prop)
		field := map[string]any{
			"name":    name,
			"type":    fieldType,
			"require": containsString(required, name),
		}
		if desc := prop.Get("description"); desc != nil && desc.Truthy() {
			field["desc"] = desc.ScalarString()
		}
		if enumValues := prop.Get("enum"); enumValues != nil && enumValues.IsArrayLike() {
			field["enum"] = nodeValues(enumValues)
		}
		if nullable := prop.Get("nullable"); (nullable != nil && nullable.Truthy()) ||
			(prop.Get("type") != nil && prop.Get("type").IsArray()) {
			field["require"] = false
			ctx.Warning("field "+name+" is nullable or a union type; marked optional", 0, 0)
		}
		if fieldType == "array" {
			items := prop.Get("items")
			if items == nil || !items.IsArrayLike() {
				items = Object()
			}
			children := SchemaFields(items, ctx, refs, depth+1)
			if len(children) > 0 {
				field["children"] = children
			}
		} else if fieldType == "object" {
			children := SchemaFields(prop, ctx, refs, depth+1)
			if len(children) > 0 {
				field["children"] = children
			}
		}
		result = append(result, field)
	}
	return result
}

// SchemaTypeOf maps JSON Schema types to UIC field types.
func SchemaTypeOf(schema *Node) string {
	typeNode := schema.Get("type")
	if typeNode != nil && typeNode.IsArray() {
		for _, item := range typeNode.Arr {
			if kind, ok := item.AsStr(); !ok || kind != "null" {
				typeNode = item
				break
			}
		}
	}
	kind, _ := typeNode.AsStr()
	switch kind {
	case "string", "integer", "number", "boolean", "array", "object":
		return kind
	case "int":
		return "integer"
	case "bool":
		return "boolean"
	}
	if properties := schema.Get("properties"); properties != nil && properties.Truthy() {
		return "object"
	}
	return "string"
}

func flattenSchema(schema *Node, ctx *Context, refs *RefCtx, seen map[string]bool, depth int) *Node {
	schema = schema.Clone()
	for {
		refstr, ok := schema.Get("$ref").AsStr()
		if !ok {
			break
		}
		if seen[refstr] || depth >= maxDepth {
			ctx.Warning("$ref cycle or depth limit detected: "+refstr, 0, 0)
			return Object()
		}
		seen[refstr] = true
		resolved := refs.Resolve(refstr, ctx)
		if resolved == nil {
			return Object()
		}
		siblings := schema.Clone()
		siblings.Remove("$ref")
		schema = Merge(siblings, resolved)
	}
	if branches := schema.Get("allOf"); branches != nil && branches.Truthy() {
		for _, branchEntry := range entries(branches) {
			branch := branchEntry.Value
			if !branch.IsArrayLike() {
				continue
			}
			branchSeen := cloneSet(seen)
			branch = flattenSchema(branch, ctx, refs, branchSeen, depth)
			if properties := branch.Get("properties"); properties != nil && properties.Truthy() {
				base := schema.Get("properties")
				if base == nil {
					base = Object()
				}
				schema.Set("properties", Merge(base, properties))
			}
			if required := branch.Get("required"); required != nil && required.Truthy() {
				merged := ArrayNode()
				if list, ok := schema.Get("required").AsArray(); ok {
					for _, item := range list {
						merged.Push(item)
					}
				}
				if list, ok := required.AsArray(); ok {
					for _, item := range list {
						merged.Push(item)
					}
				}
				schema.Set("required", merged)
			}
		}
		schema.Set("required", uniqueRequired(schema.Get("required")))
		schema.Remove("allOf")
	}
	for _, keyword := range []string{"oneOf", "anyOf"} {
		value := schema.Get(keyword)
		if value == nil || !value.Truthy() {
			continue
		}
		ctx.Warning(keyword+" is not fully supported; using the first branch", 0, 0)
		branchSeen := cloneSet(seen)
		branch := Object()
		if list, ok := value.AsArray(); ok && len(list) > 0 && list[0].IsArrayLike() {
			branch = list[0]
		}
		schema.Remove(keyword)
		branch = flattenSchema(branch, ctx, refs, branchSeen, depth)
		schema = Merge(schema, branch)
		if schema.Has("$ref") {
			schema = flattenSchema(schema, ctx, refs, branchSeen, depth)
		}
	}
	return schema
}

func uniqueRequired(required *Node) *Node {
	out := ArrayNode()
	var seen []*Node
	if list, ok := required.AsArray(); ok {
		for _, item := range list {
			duplicate := false
			for _, previous := range seen {
				if previous.StrictEq(item) {
					duplicate = true
					break
				}
			}
			if !duplicate {
				seen = append(seen, item)
				out.Push(item)
			}
		}
	}
	return out
}

func cloneSet(set map[string]bool) map[string]bool {
	out := make(map[string]bool, len(set))
	for key, value := range set {
		out[key] = value
	}
	return out
}

// maxResolvedFields bounds recursive EDMX/WSDL field expansion. It is a var so
// tests can exercise the cap with tiny graphs.
var maxResolvedFields = 10_000

// expansionBudget is shared by the request and response of one definition so a
// malicious schema cannot multiply output beyond the input cap.
type expansionBudget struct {
	remaining int
	reported  bool
}

func newExpansionBudget() *expansionBudget {
	return &expansionBudget{remaining: maxResolvedFields}
}

func (b *expansionBudget) take(ctx *Context) bool {
	if b.remaining > 0 {
		b.remaining--
		return true
	}
	if !b.reported {
		b.reported = true
		ctx.Error("definition expansion exceeds max_fields", 0, 0)
	}
	return false
}

func containsString(list []string, value string) bool {
	for _, item := range list {
		if item == value {
			return true
		}
	}
	return false
}

type schemaFailure struct {
	Path    string
	Message string
}

// ValidateSchema validates data against schema for sample checking.
func ValidateSchema(schema, data *Node, refs *RefCtx, ctx *Context, path string, depth int) []schemaFailure {
	if depth > 32 {
		return nil
	}
	var failures []schemaFailure
	schema = schema.Clone()
	seen := map[string]bool{}
	refDepth := 0
	for {
		refstr, ok := schema.Get("$ref").AsStr()
		if !ok || seen[refstr] || refDepth >= maxDepth {
			break
		}
		seen[refstr] = true
		resolved := refs.Resolve(refstr, ctx)
		if resolved == nil {
			return failures
		}
		siblings := schema.Clone()
		siblings.Remove("$ref")
		schema = Merge(siblings, resolved)
		refDepth++
	}
	if branches := schema.Get("allOf"); branches != nil && branches.Truthy() {
		for _, branch := range entries(branches) {
			if branch.Value.IsArrayLike() {
				failures = append(failures, ValidateSchema(branch.Value, data, refs, ctx, path, depth+1)...)
			}
		}
	}
	if branches := schema.Get("anyOf"); branches != nil && branches.Truthy() {
		matched := false
		for _, branch := range entries(branches) {
			if len(ValidateSchema(branch.Value, data, refs, ctx, path, depth+1)) == 0 {
				matched = true
				break
			}
		}
		if !matched {
			failures = append(failures, schemaFailure{Path: path, Message: "does not match any of the allowed schemas"})
		}
	}
	if branches := schema.Get("oneOf"); branches != nil && branches.Truthy() {
		matches := 0
		for _, branch := range entries(branches) {
			if len(ValidateSchema(branch.Value, data, refs, ctx, path, depth+1)) == 0 {
				matches++
			}
		}
		if matches != 1 {
			failures = append(failures, schemaFailure{Path: path, Message: "must match exactly one schema"})
		}
	}
	if enumValues := schema.Get("enum"); enumValues != nil && enumValues.IsArrayLike() {
		dataValue := data.ToAny()
		matched := false
		for _, item := range entries(enumValues) {
			if reflect.DeepEqual(item.Value.ToAny(), dataValue) {
				matched = true
				break
			}
		}
		if !matched {
			failures = append(failures, schemaFailure{Path: path, Message: "value is not one of the allowed values"})
		}
	}
	if expected := schema.Get("const"); expected != nil && !data.LooseEq(expected) {
		failures = append(failures, schemaFailure{Path: path, Message: "value must equal the const value"})
	}
	if typeNode := schema.Get("type"); typeNode != nil && typeNode.Kind != nullNode {
		if !matchesType(typeNode, data) {
			expected := typeNode.ScalarString()
			if typeNode.IsArray() {
				parts := make([]string, len(typeNode.Arr))
				for i, item := range typeNode.Arr {
					parts[i] = item.ScalarString()
				}
				expected = strings.Join(parts, "|")
			}
			failures = append(failures, schemaFailure{Path: path, Message: "expected type " + expected})
			return failures
		}
	}
	if !data.IsArrayLike() {
		return failures
	}
	isObject := false
	switch data.Kind {
	case arrayNode:
		isObject = len(data.Arr) == 0
	case objectNode:
		isObject = !data.IsList() || data.IsEmpty()
	}
	if isObject {
		if required, ok := schema.Get("required").AsArray(); ok {
			for _, item := range required {
				name := item.ScalarString()
				if data.Get(name) == nil {
					failures = append(failures, schemaFailure{Path: childPath(path, name), Message: "is required"})
				}
			}
		}
	}
	if isObject {
		if properties := schema.Get("properties"); properties != nil && properties.Truthy() {
			for _, property := range entries(properties) {
				if property.Value.IsArrayLike() {
					if value := data.Get(property.Key); value != nil {
						failures = append(failures, ValidateSchema(property.Value, value, refs, ctx, childPath(path, property.Key), depth+1)...)
					}
				}
			}
		}
	}
	if items := schema.Get("items"); items != nil && items.IsArrayLike() && data.IsList() {
		for _, item := range entries(data) {
			failures = append(failures, ValidateSchema(items, item.Value, refs, ctx, fmt.Sprintf("%s[%s]", path, item.Key), depth+1)...)
		}
	}
	return failures
}

func matchesType(typeNode, data *Node) bool {
	switch typeNode.Kind {
	case arrayNode:
		for _, item := range typeNode.Arr {
			if matchesOne(item.ScalarString(), data) {
				return true
			}
		}
		return false
	case objectNode:
		for _, item := range typeNode.Obj {
			if matchesOne(item.Value.ScalarString(), data) {
				return true
			}
		}
		return false
	}
	return matchesOne(typeNode.ScalarString(), data)
}

func matchesOne(kind string, data *Node) bool {
	switch kind {
	case "object":
		return data.IsArrayLike() && (data.IsEmpty() || !data.IsList())
	case "array":
		return data.IsArrayLike() && data.IsList()
	case "string":
		return data.Kind == stringNode
	case "integer":
		return data.Kind == intNode
	case "number":
		return data.Kind == intNode || data.Kind == floatNode
	case "boolean":
		return data.Kind == boolNode
	case "null":
		return data.Kind == nullNode
	}
	return true
}

func childPath(path, name string) string {
	if path == "$" {
		return "$." + name
	}
	return path + "." + name
}

// Clone returns a deep copy.
func (n *Node) Clone() *Node {
	if n == nil {
		return nil
	}
	out := &Node{Kind: n.Kind, Bool: n.Bool, Int: n.Int, Num: n.Num, Str: n.Str}
	if n.Arr != nil {
		out.Arr = make([]*Node, len(n.Arr))
		for i, item := range n.Arr {
			out.Arr[i] = item.Clone()
		}
	}
	if n.Obj != nil {
		out.Obj = make([]Member, len(n.Obj))
		for i, member := range n.Obj {
			out.Obj[i] = Member{Key: member.Key, Value: member.Value.Clone()}
		}
	}
	return out
}

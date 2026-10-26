package protocol

import (
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Node is an insertion-ordered JSON/YAML value. PHP associative arrays keep
// insertion order and the adapters emit fields in document order.
type Node struct {
	Kind nodeKind
	Bool bool
	Int  int64
	Num  float64
	Str  string
	Arr  []*Node
	Obj  []Member
}

type nodeKind uint8

const (
	nullNode nodeKind = iota
	boolNode
	intNode
	floatNode
	stringNode
	arrayNode
	objectNode
)

// Member is one key/value pair of an object node.
type Member struct {
	Key   string
	Value *Node
}

func Object() *Node             { return &Node{Kind: objectNode} }
func ArrayNode() *Node          { return &Node{Kind: arrayNode} }
func StringNode(s string) *Node { return &Node{Kind: stringNode, Str: s} }
func BoolNode(b bool) *Node     { return &Node{Kind: boolNode, Bool: b} }
func IntNode(n int64) *Node     { return &Node{Kind: intNode, Int: n} }
func FloatNode(n float64) *Node { return &Node{Kind: floatNode, Num: n} }
func NullNode() *Node           { return &Node{Kind: nullNode} }

func (n *Node) Get(key string) *Node {
	if n == nil || n.Kind != objectNode {
		return nil
	}
	for i := range n.Obj {
		if n.Obj[i].Key == key {
			return n.Obj[i].Value
		}
	}
	return nil
}

func (n *Node) GetMut(key string) *Node {
	if n == nil || n.Kind != objectNode {
		return nil
	}
	for i := range n.Obj {
		if n.Obj[i].Key == key {
			return n.Obj[i].Value
		}
	}
	return nil
}

func (n *Node) Has(key string) bool { return n.Get(key) != nil }

// Set mirrors PHP array assignment: replace in place, otherwise append.
func (n *Node) Set(key string, value *Node) {
	if n == nil || n.Kind != objectNode {
		return
	}
	for i := range n.Obj {
		if n.Obj[i].Key == key {
			n.Obj[i].Value = value
			return
		}
	}
	n.Obj = append(n.Obj, Member{Key: key, Value: value})
}

func (n *Node) Remove(key string) *Node {
	if n == nil || n.Kind != objectNode {
		return nil
	}
	for i := range n.Obj {
		if n.Obj[i].Key == key {
			value := n.Obj[i].Value
			n.Obj = append(n.Obj[:i], n.Obj[i+1:]...)
			return value
		}
	}
	return nil
}

func (n *Node) Push(value *Node) {
	if n != nil && n.Kind == arrayNode {
		n.Arr = append(n.Arr, value)
	}
}

func (n *Node) AsStr() (string, bool) {
	if n != nil && n.Kind == stringNode {
		return n.Str, true
	}
	return "", false
}

func (n *Node) AsArray() ([]*Node, bool) {
	if n != nil && n.Kind == arrayNode {
		return n.Arr, true
	}
	return nil, false
}

func (n *Node) AsObject() ([]Member, bool) {
	if n != nil && n.Kind == objectNode {
		return n.Obj, true
	}
	return nil, false
}

func (n *Node) IsObject() bool { return n != nil && n.Kind == objectNode }
func (n *Node) IsArray() bool  { return n != nil && n.Kind == arrayNode }

// IsArrayLike is PHP is_array(): object or list.
func (n *Node) IsArrayLike() bool {
	return n != nil && (n.Kind == objectNode || n.Kind == arrayNode)
}

// IsList mirrors PHP array_is_list(): any list array, and an empty object.
func (n *Node) IsList() bool {
	if n == nil {
		return false
	}
	switch n.Kind {
	case arrayNode:
		return true
	case objectNode:
		if len(n.Obj) == 0 {
			return true
		}
		for i, member := range n.Obj {
			if member.Key != strconv.Itoa(i) {
				return false
			}
		}
		return true
	}
	return false
}

func (n *Node) Keys() []string {
	if n == nil || n.Kind != objectNode {
		return nil
	}
	keys := make([]string, len(n.Obj))
	for i, member := range n.Obj {
		keys[i] = member.Key
	}
	return keys
}

func (n *Node) Len() int {
	if n == nil {
		return 0
	}
	switch n.Kind {
	case objectNode:
		return len(n.Obj)
	case arrayNode:
		return len(n.Arr)
	case stringNode:
		return len(n.Str)
	}
	return 0
}

func (n *Node) IsEmpty() bool { return n.Len() == 0 }

// ScalarString is PHP (string)$value for the scalar cases the adapters use.
func (n *Node) ScalarString() string {
	if n == nil {
		return ""
	}
	switch n.Kind {
	case nullNode:
		return ""
	case boolNode:
		if n.Bool {
			return "1"
		}
		return ""
	case intNode:
		return strconv.FormatInt(n.Int, 10)
	case floatNode:
		return formatFloat(n.Num)
	case stringNode:
		return n.Str
	}
	return "Array"
}

// VersionString is PHP var_export(float, true) for version numbers: 2.0 stays
// "2.0" instead of PHP's (string)2.0 == "2".
func (n *Node) VersionString() string {
	if n != nil && n.Kind == floatNode {
		return versionFloat(n.Num)
	}
	return n.ScalarString()
}

func formatFloat(f float64) string {
	return strconv.FormatFloat(f, 'g', -1, 64)
}

func versionFloat(f float64) string {
	if f == float64(int64(f)) && f >= -1e15 && f <= 1e15 {
		return strconv.FormatFloat(f, 'f', 1, 64)
	}
	return formatFloat(f)
}

// ToAny converts to a JSON-safe value.
func (n *Node) ToAny() any {
	if n == nil {
		return nil
	}
	switch n.Kind {
	case nullNode:
		return nil
	case boolNode:
		return n.Bool
	case intNode:
		return n.Int
	case floatNode:
		return n.Num
	case stringNode:
		return n.Str
	case arrayNode:
		items := make([]any, len(n.Arr))
		for i, item := range n.Arr {
			items[i] = item.ToAny()
		}
		return items
	case objectNode:
		obj := make(map[string]any, len(n.Obj))
		for _, member := range n.Obj {
			obj[member.Key] = member.Value.ToAny()
		}
		return obj
	}
	return nil
}

// Merge is array_merge for string-keyed objects: later keys overwrite, new keys
// append.
func Merge(base, overlay *Node) *Node {
	out := Object()
	if base != nil && base.Kind == objectNode {
		out.Obj = append(out.Obj, base.Obj...)
	}
	if overlay != nil && overlay.Kind == objectNode {
		for _, member := range overlay.Obj {
			out.Set(member.Key, member.Value)
		}
	}
	return out
}

// Truthy is PHP empty() negation for the cases the adapters rely on.
func (n *Node) Truthy() bool {
	if n == nil {
		return false
	}
	switch n.Kind {
	case nullNode:
		return false
	case boolNode:
		return n.Bool
	case intNode:
		return n.Int != 0
	case floatNode:
		return n.Num != 0
	case stringNode:
		return n.Str != "" && n.Str != "0"
	case arrayNode:
		return len(n.Arr) > 0
	case objectNode:
		return len(n.Obj) > 0
	}
	return false
}

func (n *Node) StrictEq(other *Node) bool {
	if n == nil || other == nil {
		return n == other
	}
	if n.Kind != other.Kind {
		return false
	}
	switch n.Kind {
	case nullNode:
		return true
	case boolNode:
		return n.Bool == other.Bool
	case intNode:
		return n.Int == other.Int
	case floatNode:
		return n.Num == other.Num
	case stringNode:
		return n.Str == other.Str
	case arrayNode:
		if len(n.Arr) != len(other.Arr) {
			return false
		}
		for i := range n.Arr {
			if !n.Arr[i].StrictEq(other.Arr[i]) {
				return false
			}
		}
		return true
	case objectNode:
		if len(n.Obj) != len(other.Obj) {
			return false
		}
		for i := range n.Obj {
			if n.Obj[i].Key != other.Obj[i].Key || !n.Obj[i].Value.StrictEq(other.Obj[i].Value) {
				return false
			}
		}
		return true
	}
	return false
}

// LooseEq is PHP loose equality for const checks.
func (n *Node) LooseEq(other *Node) bool {
	if n == nil || other == nil {
		if n == nil && other == nil {
			return true
		}
		return false
	}
	switch {
	case n.Kind == intNode && other.Kind == floatNode:
		return float64(n.Int) == other.Num
	case n.Kind == floatNode && other.Kind == intNode:
		return n.Num == float64(other.Int)
	case n.Kind == stringNode && other.Kind == intNode:
		value, err := strconv.ParseInt(strings.TrimSpace(n.Str), 10, 64)
		return err == nil && value == other.Int
	case n.Kind == intNode && other.Kind == stringNode:
		value, err := strconv.ParseInt(strings.TrimSpace(other.Str), 10, 64)
		return err == nil && value == n.Int
	case n.Kind == stringNode && other.Kind == floatNode:
		value, err := strconv.ParseFloat(strings.TrimSpace(n.Str), 64)
		return err == nil && value == other.Num
	case n.Kind == floatNode && other.Kind == stringNode:
		value, err := strconv.ParseFloat(strings.TrimSpace(other.Str), 64)
		return err == nil && value == n.Num
	case n.Kind == nullNode && other.Kind == stringNode:
		return other.Str == ""
	case n.Kind == stringNode && other.Kind == nullNode:
		return n.Str == ""
	}
	return n.StrictEq(other)
}

var errJSONDepth = errors.New("json depth")

// ParseJSON decodes content into an ordered Node. Error strings match PHP
// json_last_error_msg for the cases the adapters distinguish.
func ParseJSON(content string) (*Node, string) {
	if !utf8.ValidString(content) {
		return nil, "Malformed UTF-8 characters, possibly incorrectly encoded"
	}
	dec := json.NewDecoder(strings.NewReader(content))
	dec.UseNumber()
	node, err := decodeJSONValue(dec, 0)
	if err != nil {
		if errors.Is(err, errJSONDepth) {
			return nil, "Maximum stack depth exceeded"
		}
		return nil, "Syntax error"
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, "Syntax error"
	}
	return node, ""
}

func decodeJSONValue(dec *json.Decoder, depth int) (*Node, error) {
	if depth > 512 {
		return nil, errJSONDepth
	}
	token, err := dec.Token()
	if err != nil {
		return nil, err
	}
	switch value := token.(type) {
	case json.Delim:
		switch value {
		case '{':
			node := Object()
			for dec.More() {
				keyToken, err := dec.Token()
				if err != nil {
					return nil, err
				}
				key, ok := keyToken.(string)
				if !ok {
					return nil, errors.New("bad object key")
				}
				child, err := decodeJSONValue(dec, depth+1)
				if err != nil {
					return nil, err
				}
				node.Set(key, child)
			}
			if _, err := dec.Token(); err != nil {
				return nil, err
			}
			return node, nil
		case '[':
			node := ArrayNode()
			for dec.More() {
				child, err := decodeJSONValue(dec, depth+1)
				if err != nil {
					return nil, err
				}
				node.Push(child)
			}
			if _, err := dec.Token(); err != nil {
				return nil, err
			}
			return node, nil
		}
		return nil, errors.New("unexpected delimiter")
	case string:
		return StringNode(value), nil
	case bool:
		return BoolNode(value), nil
	case nil:
		return NullNode(), nil
	case json.Number:
		text := value.String()
		if !strings.ContainsAny(text, ".eE") {
			if integer, err := strconv.ParseInt(text, 10, 64); err == nil {
				return IntNode(integer), nil
			}
		}
		number, err := strconv.ParseFloat(text, 64)
		if err != nil {
			return nil, err
		}
		return FloatNode(number), nil
	}
	return nil, errors.New("unexpected token")
}

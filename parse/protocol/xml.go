package protocol

import (
	"encoding/xml"
	"io"
	"strings"
	"unicode/utf8"
)

const (
	maxXMLDepth = 256
	maxXMLNodes = 200_000
)

// XMLNode is a minimal safe DOM node; Name is the local name (prefix stripped),
// matching DOM localName.
type XMLNode struct {
	Name     string
	Attrs    []XMLAttr
	Children []*XMLNode
	Text     string
}

type XMLAttr struct {
	Key, Value string
}

type XMLDoc struct {
	Root *XMLNode
}

func (n *XMLNode) Attr(name string) string {
	for _, attr := range n.Attrs {
		if attr.Key == name {
			return attr.Value
		}
	}
	return ""
}

func (n *XMLNode) First(local string) *XMLNode {
	for _, child := range n.Children {
		if child.Name == local {
			return child
		}
	}
	return nil
}

func (n *XMLNode) All(local string) []*XMLNode {
	var result []*XMLNode
	for _, child := range n.Children {
		if child.Name == local {
			result = append(result, child)
		}
	}
	return result
}

func (n *XMLNode) FirstElement() *XMLNode {
	if len(n.Children) == 0 {
		return nil
	}
	return n.Children[0]
}

func (n *XMLNode) Elements() []*XMLNode { return n.Children }

func (n *XMLNode) TextContent() string {
	var text strings.Builder
	text.WriteString(n.Text)
	for _, child := range n.Children {
		text.WriteString(child.TextContent())
	}
	return text.String()
}

// LoadXML parses untrusted XML. DOCTYPE is rejected and entities are never
// expanded beyond XML's predefined/numeric references.
func LoadXML(content string) (*XMLDoc, *ParseError) {
	if strings.Contains(strings.ToLower(content), "<!doctype") {
		return nil, &ParseError{Message: "DOCTYPE is not allowed"}
	}
	decoder := xml.NewDecoder(strings.NewReader(content))
	var stack []*XMLNode
	var root *XMLNode
	nodes := 0
	for {
		offset := decoder.InputOffset()
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, xmlError(content, offset, err.Error())
		}
		switch value := token.(type) {
		case xml.StartElement:
			nodes++
			if nodes > maxXMLNodes {
				return nil, xmlError(content, offset, "XML node limit exceeded")
			}
			if len(stack) >= maxXMLDepth {
				return nil, xmlError(content, offset, "XML nesting limit exceeded")
			}
			if len(stack) == 0 && root != nil {
				return nil, xmlError(content, offset, "multiple root elements")
			}
			stack = append(stack, xmlElement(value))
		case xml.EndElement:
			if len(stack) == 0 {
				return nil, xmlError(content, offset, "unexpected closing tag")
			}
			node := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if len(stack) == 0 {
				root = node
			} else {
				parent := stack[len(stack)-1]
				parent.Children = append(parent.Children, node)
			}
		case xml.CharData:
			text := string(value)
			if len(stack) == 0 {
				if strings.TrimSpace(text) != "" {
					return nil, xmlError(content, offset, "content outside the root element")
				}
			} else {
				stack[len(stack)-1].Text += text
			}
		case xml.Directive:
			directive := strings.TrimSpace(string(value))
			if strings.HasPrefix(strings.ToLower(directive), "doctype") {
				return nil, &ParseError{Message: "DOCTYPE is not allowed"}
			}
		}
	}
	if len(stack) != 0 {
		return nil, xmlError(content, decoder.InputOffset(), "unclosed element")
	}
	if root == nil {
		return nil, &ParseError{
			Message: "DOMDocument::loadXML(): Argument #1 ($source) must not be empty",
		}
	}
	return &XMLDoc{Root: root}, nil
}

func xmlElement(start xml.StartElement) *XMLNode {
	node := &XMLNode{Name: start.Name.Local}
	for _, attr := range start.Attr {
		node.Attrs = append(node.Attrs, XMLAttr{Key: attr.Name.Local, Value: attr.Value})
	}
	return node
}

func xmlError(content string, offset int64, message string) *ParseError {
	index := int(offset)
	if index >= len(content) {
		// InputOffset is exactly len(content) for an unexpected EOF; index the
		// whole prefix instead of content[len(content)].
		index = len(content)
	} else {
		for index > 0 && !utf8.RuneStart(content[index]) {
			index--
		}
	}
	prefix := content[:index]
	line := int64(strings.Count(prefix, "\n")) + 1
	last := prefix
	if position := strings.LastIndex(prefix, "\n"); position >= 0 {
		last = prefix[position+1:]
	}
	column := int64(utf8.RuneCountInString(last)) + 1
	return &ParseError{
		Message: "XML parse error: " + message,
		Line:    line,
		Column:  column,
	}
}

// LocalName strips a namespace prefix.
func LocalName(qualified string) string {
	if index := strings.LastIndexByte(qualified, ':'); index >= 0 {
		return qualified[index+1:]
	}
	return qualified
}

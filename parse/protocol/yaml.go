package protocol

import (
	"strconv"
	"strings"
)

// YAML subset ported from php/src/protocol/Support/Yaml.php. Anchors, aliases,
// tags, multiple documents and tab indentation are rejected.

type yamlLine struct {
	indent  int
	content string
	line    int64
}

func yamlError(message string, line int64) *ParseError {
	return &ParseError{Message: "YAML: " + message, Line: line}
}

func parseYAML(source string) (*Node, *ParseError) {
	lines, err := yamlLines(source)
	if err != nil {
		return nil, err
	}
	if len(lines) == 0 {
		return Object(), nil
	}
	index := 0
	result, err := parseYAMLBlock(lines, &index, lines[0].indent, 0)
	if err != nil {
		return nil, err
	}
	if index < len(lines) {
		return nil, yamlError("unexpected content", lines[index].line)
	}
	if result != nil && result.IsArrayLike() {
		return result, nil
	}
	return Object(), nil
}

func yamlLines(source string) ([]yamlLine, *ParseError) {
	source = strings.ReplaceAll(strings.ReplaceAll(source, "\r\n", "\n"), "\r", "\n")
	var lines []yamlLine
	for number, raw := range strings.Split(source, "\n") {
		spaces := 0
		for spaces < len(raw) && raw[spaces] == ' ' {
			spaces++
		}
		if spaces < len(raw) && raw[spaces] == '\t' {
			return nil, yamlError("tab indentation is not supported", int64(number+1))
		}
		trimmed := phpTrim(raw)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if trimmed == "---" || trimmed == "..." {
			return nil, yamlError("multi-document YAML is not supported", int64(number+1))
		}
		lines = append(lines, yamlLine{
			indent:  spaces,
			content: phpTrimRight(raw[spaces:]),
			line:    int64(number + 1),
		})
	}
	return lines, nil
}

func parseYAMLBlock(lines []yamlLine, index *int, indent, depth int) (*Node, *ParseError) {
	if depth > 512 {
		var line int64
		if *index < len(lines) {
			line = lines[*index].line
		}
		return nil, yamlError("nesting limit reached", line)
	}
	if *index >= len(lines) || lines[*index].indent < indent {
		return nil, nil
	}
	if lines[*index].indent > indent {
		return nil, yamlError("bad indentation", lines[*index].line)
	}
	if isYAMLSequence(lines[*index].content) {
		return parseYAMLSequence(lines, index, indent, depth)
	}
	return parseYAMLMapping(lines, index, indent, depth)
}

func parseYAMLMapping(lines []yamlLine, index *int, indent, depth int) (*Node, *ParseError) {
	result := Object()
	for *index < len(lines) && lines[*index].indent == indent {
		line := lines[*index]
		if isYAMLSequence(line.content) {
			return nil, yamlError("unexpected sequence item", line.line)
		}
		key, value, err := yamlSplitKey(line.content, line.line)
		if err != nil {
			return nil, err
		}
		*index++
		var node *Node
		if value == "" {
			if *index < len(lines) && lines[*index].indent > indent {
				childIndent := lines[*index].indent
				node, err = parseYAMLBlock(lines, index, childIndent, depth+1)
				if err != nil {
					return nil, err
				}
				if node == nil {
					node = NullNode()
				}
			} else {
				node = NullNode()
			}
		} else if value == "|" || value == ">" {
			node = StringNode(parseYAMLBlockScalar(lines, index, indent, value))
		} else {
			node, err = parseYAMLScalar(value, line.line, depth+1)
			if err != nil {
				return nil, err
			}
		}
		result.Set(key, node)
	}
	if *index < len(lines) && lines[*index].indent > indent {
		return nil, yamlError("bad indentation", lines[*index].line)
	}
	return result, nil
}

func parseYAMLSequence(lines []yamlLine, index *int, indent, depth int) (*Node, *ParseError) {
	result := ArrayNode()
	for *index < len(lines) && lines[*index].indent == indent && isYAMLSequence(lines[*index].content) {
		line := lines[*index]
		rest := phpTrimLeft(line.content[1:])
		*index++
		if rest == "" {
			if *index < len(lines) && lines[*index].indent > indent {
				childIndent := lines[*index].indent
				child, err := parseYAMLBlock(lines, index, childIndent, depth+1)
				if err != nil {
					return nil, err
				}
				if child == nil {
					child = NullNode()
				}
				result.Push(child)
			} else {
				result.Push(NullNode())
			}
			continue
		}
		if yamlLooksLikeKey(rest) {
			key, value, err := yamlSplitKey(rest, line.line)
			if err != nil {
				return nil, err
			}
			item := Object()
			var node *Node
			if value == "" {
				if *index < len(lines) && lines[*index].indent > indent {
					childIndent := lines[*index].indent
					node, err = parseYAMLBlock(lines, index, childIndent, depth+1)
					if err != nil {
						return nil, err
					}
					if node == nil {
						node = NullNode()
					}
				} else {
					node = NullNode()
				}
			} else if value == "|" || value == ">" {
				node = StringNode(parseYAMLBlockScalar(lines, index, indent, value))
			} else {
				node, err = parseYAMLScalar(value, line.line, depth+1)
				if err != nil {
					return nil, err
				}
			}
			item.Set(key, node)
			if *index < len(lines) && lines[*index].indent > indent {
				extraIndent := lines[*index].indent
				extra, err := parseYAMLMapping(lines, index, extraIndent, depth+1)
				if err != nil {
					return nil, err
				}
				for _, member := range extra.Obj {
					item.Set(member.Key, member.Value)
				}
			}
			result.Push(item)
		} else {
			node, err := parseYAMLScalar(rest, line.line, depth+1)
			if err != nil {
				return nil, err
			}
			result.Push(node)
			if *index < len(lines) && lines[*index].indent > indent {
				return nil, yamlError("bad indentation", lines[*index].line)
			}
		}
	}
	return result, nil
}

func parseYAMLBlockScalar(lines []yamlLine, index *int, parentIndent int, style string) string {
	var parts []string
	minIndent := -1
	start := *index
	for *index < len(lines) && lines[*index].indent > parentIndent {
		line := lines[*index]
		if minIndent < 0 {
			minIndent = line.indent
		}
		parts = append(parts, strings.Repeat(" ", max(line.indent-minIndent, 0))+line.content)
		*index++
	}
	if *index == start {
		return ""
	}
	if style == "|" {
		return strings.Join(parts, "\n")
	}
	return strings.Join(parts, " ")
}

func isYAMLSequence(content string) bool {
	return content == "-" || strings.HasPrefix(content, "- ")
}

func yamlLooksLikeKey(content string) bool {
	_, ok := findTopLevelColon(content)
	return ok
}

func yamlSplitKey(content string, line int64) (string, string, *ParseError) {
	pos, ok := findTopLevelColon(content)
	if !ok {
		return "", "", yamlError(`expected "key: value"`, line)
	}
	rawKey := phpTrim(content[:pos])
	if rawKey == "" {
		return "", "", yamlError("empty key", line)
	}
	key := rawKey
	if isYAMLQuoted(rawKey) {
		node, err := parseYAMLScalar(rawKey, line, 0)
		if err != nil {
			return "", "", err
		}
		key = node.ScalarString()
	}
	if strings.HasPrefix(key, "&") || strings.HasPrefix(key, "*") || strings.HasPrefix(key, "!") {
		return "", "", yamlError("anchors, aliases and tags are not supported", line)
	}
	return key, phpTrim(content[pos+1:]), nil
}

func findTopLevelColon(content string) (int, bool) {
	var quote byte
	depth := 0
	for i := 0; i < len(content); i++ {
		ch := content[i]
		if quote != 0 {
			if ch == '\\' {
				i++
				continue
			}
			if ch == quote {
				quote = 0
			}
			continue
		}
		switch ch {
		case '"', '\'':
			quote = ch
		case '[', '{':
			depth++
		case ']', '}':
			if depth > 0 {
				depth--
			}
		case ':':
			if depth == 0 && (i+1 >= len(content) || content[i+1] == ' ' || content[i+1] == '\t') {
				return i, true
			}
		}
	}
	return 0, false
}

func parseYAMLScalar(value string, line int64, depth int) (*Node, *ParseError) {
	if depth > 256 {
		return nil, yamlError("nesting limit reached", line)
	}
	value = phpTrim(stripYAMLComment(value))
	if value == "" {
		return StringNode(""), nil
	}
	if isYAMLQuoted(value) {
		return StringNode(unquoteYAML(value)), nil
	}
	switch value[0] {
	case '&', '*', '!':
		return nil, yamlError("anchors, aliases and tags are not supported", line)
	case '[':
		return parseYAMLInlineList(value, line, depth+1)
	case '{':
		return parseYAMLInlineMap(value, line, depth+1)
	}
	lower := strings.ToLower(value)
	if lower == "null" || value == "~" {
		return NullNode(), nil
	}
	if lower == "true" || lower == "false" {
		return BoolNode(lower == "true"), nil
	}
	if isYAMLInteger(value) {
		if integer, err := strconv.ParseInt(value, 10, 64); err == nil {
			return IntNode(integer), nil
		}
	}
	if isYAMLFloat(value) {
		if number, err := strconv.ParseFloat(value, 64); err == nil {
			return FloatNode(number), nil
		}
	}
	return StringNode(value), nil
}

// inlineYAMLInner is PHP substr($value, 1, -1): truncated input yields "".
func inlineYAMLInner(value string) string {
	if len(value) < 2 {
		return ""
	}
	return phpTrim(value[1 : len(value)-1])
}

func parseYAMLInlineList(value string, line int64, depth int) (*Node, *ParseError) {
	inner := inlineYAMLInner(value)
	if inner == "" {
		return ArrayNode(), nil
	}
	result := ArrayNode()
	for _, item := range splitYAMLInline(inner) {
		node, err := parseYAMLScalar(item, line, depth)
		if err != nil {
			return nil, err
		}
		result.Push(node)
	}
	return result, nil
}

func parseYAMLInlineMap(value string, line int64, depth int) (*Node, *ParseError) {
	inner := inlineYAMLInner(value)
	if inner == "" {
		return Object(), nil
	}
	result := Object()
	for _, pair := range splitYAMLInline(inner) {
		key, itemValue, err := yamlSplitKey(pair, line)
		if err != nil {
			return nil, err
		}
		node := NullNode()
		if itemValue != "" {
			node, err = parseYAMLScalar(itemValue, line, depth+1)
			if err != nil {
				return nil, err
			}
		}
		result.Set(key, node)
	}
	return result, nil
}

func splitYAMLInline(value string) []string {
	var parts []string
	var buffer strings.Builder
	var quote rune
	depth := 0
	runes := []rune(value)
	for i := 0; i < len(runes); i++ {
		ch := runes[i]
		if quote != 0 {
			buffer.WriteRune(ch)
			if ch == '\\' && i+1 < len(runes) {
				i++
				buffer.WriteRune(runes[i])
			} else if ch == quote {
				quote = 0
			}
			continue
		}
		switch ch {
		case '"', '\'':
			quote = ch
			buffer.WriteRune(ch)
		case '[', '{':
			depth++
			buffer.WriteRune(ch)
		case ']', '}':
			if depth > 0 {
				depth--
			}
			buffer.WriteRune(ch)
		case ',':
			if depth == 0 {
				parts = append(parts, phpTrim(buffer.String()))
				buffer.Reset()
			} else {
				buffer.WriteRune(ch)
			}
		default:
			buffer.WriteRune(ch)
		}
	}
	if trimmed := phpTrim(buffer.String()); trimmed != "" {
		parts = append(parts, trimmed)
	}
	return parts
}

func isYAMLQuoted(value string) bool {
	return len(value) >= 2 && (value[0] == '"' || value[0] == '\'') && value[len(value)-1] == value[0]
}

func unquoteYAML(value string) string {
	quote := value[0]
	inner := value[1 : len(value)-1]
	if quote == '\'' {
		return strings.ReplaceAll(inner, "''", "'")
	}
	return stripcslashes(inner)
}

func stripYAMLComment(value string) string {
	var quote byte
	for i := 0; i < len(value); i++ {
		ch := value[i]
		if quote != 0 {
			if ch == '\\' {
				i++
				continue
			}
			if ch == quote {
				quote = 0
			}
			continue
		}
		if ch == '"' || ch == '\'' {
			quote = ch
		} else if ch == '#' && (i == 0 || value[i-1] == ' ' || value[i-1] == '\t') {
			return phpTrimRight(value[:i])
		}
	}
	return value
}

func isYAMLInteger(value string) bool {
	value = strings.TrimPrefix(value, "-")
	if value == "" {
		return false
	}
	for i := 0; i < len(value); i++ {
		if value[i] < '0' || value[i] > '9' {
			return false
		}
	}
	return true
}

func isYAMLFloat(value string) bool {
	value = strings.TrimPrefix(value, "-")
	mantissa, exponent := value, ""
	if index := strings.IndexAny(value, "eE"); index >= 0 {
		mantissa, exponent = value[:index], value[index+1:]
	}
	whole, fraction := mantissa, ""
	hasFraction := false
	if index := strings.IndexByte(mantissa, '.'); index >= 0 {
		whole, fraction = mantissa[:index], mantissa[index+1:]
		hasFraction = true
	}
	if whole == "" || !allDigits(whole) {
		return false
	}
	if hasFraction && (fraction == "" || !allDigits(fraction)) {
		return false
	}
	hasExponent := exponent != ""
	if hasExponent {
		exponent = strings.TrimPrefix(strings.TrimPrefix(exponent, "+"), "-")
		if exponent == "" || !allDigits(exponent) {
			return false
		}
	}
	// PHP only accepts decimals with a fraction or an exponent.
	return hasFraction || hasExponent
}

func allDigits(value string) bool {
	for i := 0; i < len(value); i++ {
		if value[i] < '0' || value[i] > '9' {
			return false
		}
	}
	return true
}

// stripcslashes is PHP stripcslashes for the escape forms in fixtures.
func stripcslashes(value string) string {
	var out strings.Builder
	runes := []rune(value)
	for i := 0; i < len(runes); i++ {
		ch := runes[i]
		if ch != '\\' {
			out.WriteRune(ch)
			continue
		}
		if i+1 >= len(runes) {
			break
		}
		i++
		next := runes[i]
		switch next {
		case 'n':
			out.WriteByte('\n')
		case 'r':
			out.WriteByte('\r')
		case 't':
			out.WriteByte('\t')
		case 'v':
			out.WriteByte('\v')
		case 'f':
			out.WriteByte('\f')
		case '\\', '\'', '"', '$':
			out.WriteRune(next)
		case '0', '1', '2', '3', '4', '5', '6', '7':
			digits := string(next)
			for len(digits) < 3 && i+1 < len(runes) && runes[i+1] >= '0' && runes[i+1] <= '7' {
				i++
				digits += string(runes[i])
			}
			if code, err := strconv.ParseUint(digits, 8, 32); err == nil {
				out.WriteRune(rune(code))
			}
		case 'x':
			var digits strings.Builder
			for digits.Len() < 2 && i+1 < len(runes) && isHexRune(runes[i+1]) {
				i++
				digits.WriteRune(runes[i])
			}
			if code, err := strconv.ParseUint(digits.String(), 16, 32); err == nil {
				out.WriteRune(rune(code))
			}
		default:
			out.WriteRune(next)
		}
	}
	return out.String()
}

func isHexRune(ch rune) bool {
	return (ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'f') || (ch >= 'A' && ch <= 'F')
}

func phpTrim(value string) string {
	return phpTrimRight(phpTrimLeft(value))
}

func phpTrimLeft(value string) string {
	return strings.TrimLeft(value, " \t\n\r\x00\v")
}

func phpTrimRight(value string) string {
	return strings.TrimRight(value, " \t\n\r\x00\v")
}

package protocol

import (
	"strings"
)

// TokenStream is ported from php/src/protocol/Support/TokenStream.php.
type TokenStream struct {
	source []rune
	pos    int
	line   int64
	column int64
	err    *ParseError
}

func NewTokenStream(source string) *TokenStream {
	source = strings.ReplaceAll(strings.ReplaceAll(source, "\r\n", "\n"), "\r", "\n")
	return &TokenStream{source: []rune(source), line: 1, column: 1}
}

func (t *TokenStream) EOF() bool {
	t.skipTrivia()
	return t.pos >= len(t.source)
}

func (t *TokenStream) NextChar() string {
	if t.pos >= len(t.source) {
		return ""
	}
	ch := t.source[t.pos]
	t.advance()
	return string(ch)
}

func (t *TokenStream) Peek(offset int) string {
	savePos, saveLine, saveColumn := t.pos, t.line, t.column
	t.skipTrivia()
	for i := 0; i < offset; i++ {
		t.advance()
	}
	var value string
	if t.pos < len(t.source) {
		value = string(t.source[t.pos])
	}
	t.pos, t.line, t.column = savePos, saveLine, saveColumn
	return value
}

func (t *TokenStream) Consume(chars string) (rune, bool) {
	t.skipTrivia()
	if t.pos >= len(t.source) {
		return 0, false
	}
	ch := t.source[t.pos]
	if !strings.ContainsRune(chars, ch) {
		return 0, false
	}
	t.advance()
	return ch, true
}

func (t *TokenStream) Expect(expected rune) *ParseError {
	t.skipTrivia()
	if t.err != nil {
		return t.err
	}
	if t.pos >= len(t.source) || t.source[t.pos] != expected {
		return t.Errorf("expected \"" + string(expected) + "\"")
	}
	t.advance()
	return nil
}

func (t *TokenStream) PeekIdentifier() string {
	savePos, saveLine, saveColumn := t.pos, t.line, t.column
	t.skipTrivia()
	value := t.scanIdentifier()
	t.pos, t.line, t.column = savePos, saveLine, saveColumn
	return value
}

func (t *TokenStream) Identifier() (string, *ParseError) {
	t.skipTrivia()
	if t.err != nil {
		return "", t.err
	}
	value := t.scanIdentifier()
	if value == "" {
		return "", t.Errorf("expected identifier")
	}
	for range value {
		t.advance()
	}
	return value, nil
}

func (t *TokenStream) Number() (string, *ParseError) {
	t.skipTrivia()
	if t.err != nil {
		return "", t.err
	}
	start := t.pos
	length := len(t.source)
	if t.pos < length && t.source[t.pos] == '-' {
		t.advance()
	}
	digitStart := t.pos
	for t.pos < length && t.source[t.pos] >= '0' && t.source[t.pos] <= '9' {
		t.advance()
	}
	if t.pos == digitStart {
		t.reposition(start)
		return "", t.Errorf("expected number")
	}
	if t.pos+1 < length && t.source[t.pos] == '.' && t.source[t.pos+1] >= '0' && t.source[t.pos+1] <= '9' {
		t.advance()
		for t.pos < length && t.source[t.pos] >= '0' && t.source[t.pos] <= '9' {
			t.advance()
		}
	}
	if t.pos < length && (t.source[t.pos] == 'e' || t.source[t.pos] == 'E') {
		exponentStart := t.pos
		t.advance()
		if t.pos < length && (t.source[t.pos] == '+' || t.source[t.pos] == '-') {
			t.advance()
		}
		exponentDigits := t.pos
		for t.pos < length && t.source[t.pos] >= '0' && t.source[t.pos] <= '9' {
			t.advance()
		}
		if t.pos == exponentDigits {
			t.reposition(exponentStart)
		}
	}
	return string(t.source[start:t.pos]), nil
}

func (t *TokenStream) String() (string, *ParseError) {
	t.skipTrivia()
	if t.err != nil {
		return "", t.err
	}
	if t.pos >= len(t.source) {
		return "", t.Errorf("expected string")
	}
	quote := t.source[t.pos]
	if quote != '"' && quote != '\'' {
		return "", t.Errorf("expected string")
	}
	t.advance()
	var value strings.Builder
	for t.pos < len(t.source) {
		ch := t.source[t.pos]
		if ch == '\\' {
			t.advance()
			if t.pos >= len(t.source) {
				return "", t.Errorf("unterminated string escape")
			}
			value.WriteString(stripcslashes("\\" + string(t.source[t.pos])))
			t.advance()
			continue
		}
		if ch == quote {
			t.advance()
			return value.String(), nil
		}
		value.WriteRune(ch)
		t.advance()
	}
	return "", t.Errorf("unterminated string")
}

func (t *TokenStream) skipTrivia() {
	for t.pos < len(t.source) {
		ch := t.source[t.pos]
		if ch == ' ' || ch == '\t' || ch == '\n' {
			t.advance()
			continue
		}
		if ch == '#' || (ch == '/' && t.peekRaw(1) == '/') {
			for t.pos < len(t.source) && t.source[t.pos] != '\n' {
				t.advance()
			}
			continue
		}
		if ch == '/' && t.peekRaw(1) == '*' {
			t.advance()
			t.advance()
			for t.pos < len(t.source) && !(t.source[t.pos] == '*' && t.peekRaw(1) == '/') {
				t.advance()
			}
			if t.pos >= len(t.source) {
				// Truncated block comment: EOF() cannot report an error, so keep
				// it for the caller to check after the parse loop.
				if t.err == nil {
					t.err = t.Errorf("unterminated block comment")
				}
				return
			}
			t.advance()
			t.advance()
			continue
		}
		break
	}
}

func (t *TokenStream) TriviaError() *ParseError { return t.err }

func (t *TokenStream) Line() int64   { return t.line }
func (t *TokenStream) Column() int64 { return t.column }

func (t *TokenStream) Errorf(message string) *ParseError {
	return &ParseError{Message: message, Line: t.line, Column: t.column}
}

func (t *TokenStream) scanIdentifier() string {
	index := t.pos
	if index >= len(t.source) {
		return ""
	}
	first := t.source[index]
	if !isIdentifierStart(first) {
		return ""
	}
	index++
	for index < len(t.source) && isIdentifierPart(t.source[index]) {
		index++
	}
	return string(t.source[t.pos:index])
}

func isIdentifierStart(ch rune) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || ch == '_'
}

func isIdentifierPart(ch rune) bool {
	return isIdentifierStart(ch) || (ch >= '0' && ch <= '9') || ch == '.'
}

func (t *TokenStream) peekRaw(offset int) rune {
	if t.pos+offset < len(t.source) {
		return t.source[t.pos+offset]
	}
	return 0
}

func (t *TokenStream) advance() {
	if t.pos >= len(t.source) {
		return
	}
	if t.source[t.pos] == '\n' {
		t.line++
		t.column = 1
	} else {
		t.column++
	}
	t.pos++
}

func (t *TokenStream) reposition(target int) {
	line, column := int64(1), int64(1)
	limit := target
	if limit > len(t.source) {
		limit = len(t.source)
	}
	for index := 0; index < limit; index++ {
		if t.source[index] == '\n' {
			line++
			column = 1
		} else {
			column++
		}
	}
	t.line, t.column, t.pos = line, column, target
}

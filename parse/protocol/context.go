package protocol

import (
	"strings"
)

// ParseError mirrors ProtocolParseException: message plus 1-based position.
type ParseError struct {
	Message string
	Line    int64
	Column  int64
}

// Options selects the operation/message and supplies a sample. The zero value
// matches the PHP defaults: unsupported constructs are errors. Set Lenient to
// downgrade them to warnings.
type Options struct {
	Operation      string
	Message        string
	Direction      string
	Sample         string
	SampleEncoding string
	Lenient        bool
}

// Limits caps untrusted input. Zero fields take their DefaultLimits value.
type Limits struct {
	MaxFileBytes int
	MaxLines     int
	MaxImports   int
	MaxDepth     int
}

func DefaultLimits() Limits {
	return Limits{
		// PHP's default is 2 MiB; the UIC-1 Go parser tightens it to 1 MiB.
		MaxFileBytes: 1 << 20,
		MaxLines:     50_000,
		MaxImports:   64,
		MaxDepth:     32,
	}
}

func (l Limits) normalized() Limits {
	defaults := DefaultLimits()
	if l.MaxFileBytes <= 0 {
		l.MaxFileBytes = defaults.MaxFileBytes
	}
	if l.MaxLines <= 0 {
		l.MaxLines = defaults.MaxLines
	}
	if l.MaxImports <= 0 {
		l.MaxImports = defaults.MaxImports
	}
	if l.MaxDepth <= 0 {
		l.MaxDepth = defaults.MaxDepth
	}
	return l
}

// Diagnostic is one validation entry. Path is always "" for parser diagnostics.
type Diagnostic struct {
	Path     string `json:"path"`
	Line     int64  `json:"line"`
	Column   int64  `json:"column"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
}

type Validation struct {
	OK       bool         `json:"ok"`
	Errors   []Diagnostic `json:"errors"`
	Warnings []Diagnostic `json:"warnings"`
}

// SampleError is the sample-validation diagnostic shape: no line/column, same
// as the PHP/UIC contract.
type SampleError struct {
	Path     string `json:"path"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
}

type SampleValidation struct {
	OK       bool          `json:"ok"`
	Errors   []SampleError `json:"errors"`
	Warnings []SampleError `json:"warnings"`
}

func okSampleValidation() SampleValidation {
	return SampleValidation{OK: true, Errors: []SampleError{}, Warnings: []SampleError{}}
}

// Resolver loads an import or $ref target. path is normalized relative to the
// defining file. Read returning false makes the parser record the PHP
// diagnostic; Failure may explain a valid-path read failure (size cap, IO).
// The parser never touches the filesystem or network itself.
type Resolver interface {
	Read(path string) (key, content string, ok bool)
	Failure() string
}

type NoResolver struct{}

func (NoResolver) Read(string) (string, string, bool) { return "", "", false }
func (NoResolver) Failure() string                    { return "" }

type Loader int

const (
	LoaderJSON Loader = iota
	LoaderJSONOrYAML
)

// Load parses content and requires a mapping/sequence root (is_array).
func (l Loader) Load(content string) (*Node, *ParseError) {
	var node *Node
	var err *ParseError
	switch l {
	case LoaderJSON:
		message := ""
		node, message = ParseJSON(content)
		if message != "" {
			return nil, &ParseError{Message: message}
		}
	default:
		trimmed := strings.TrimLeft(content, " \t\r\n\f\v")
		if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
			message := ""
			node, message = ParseJSON(content)
			if message != "" {
				return nil, &ParseError{Message: message}
			}
		} else {
			node, err = parseYAML(content)
			if err != nil {
				return nil, err
			}
		}
	}
	if !node.IsArrayLike() {
		return nil, &ParseError{Message: "document root must be an object", Line: 1, Column: 1}
	}
	return node, nil
}

func (l Loader) LoadDocument(content string) *Node {
	node, err := l.Load(content)
	if err != nil {
		return nil
	}
	return node
}

type contextFrame struct {
	file  string
	depth int
}

type Context struct {
	file      string
	options   Options
	limits    Limits
	errors    []Diagnostic
	warnings  []Diagnostic
	imports   []string
	visited   map[string]bool
	depth     int
	stack     []contextFrame
	resolver  Resolver
	contents  map[string]string
	documents map[string]*Node
	expanded  map[string]*Node
}

func NewContext(file, content string, options Options, limits Limits, resolver Resolver) *Context {
	if resolver == nil {
		resolver = NoResolver{}
	}
	return &Context{
		file:      file,
		options:   options,
		limits:    limits.normalized(),
		errors:    []Diagnostic{},
		warnings:  []Diagnostic{},
		imports:   []string{},
		visited:   map[string]bool{file: true},
		resolver:  resolver,
		contents:  map[string]string{file: content},
		documents: map[string]*Node{},
		expanded:  map[string]*Node{},
	}
}

func (c *Context) File() string           { return c.file }
func (c *Context) Options() Options       { return c.options }
func (c *Context) Limits() Limits         { return c.limits }
func (c *Context) Errors() []Diagnostic   { return c.errors }
func (c *Context) Warnings() []Diagnostic { return c.warnings }
func (c *Context) Imports() []string      { return c.imports }

// Read returns the current document under byte/line limits.
func (c *Context) Read() (string, bool) {
	content, ok := c.contents[c.file]
	if !ok {
		c.Error("unable to read file", 0, 0)
		return "", false
	}
	if len(content) > c.limits.MaxFileBytes {
		c.Error("file exceeds max_file_bytes", 0, 0)
		return "", false
	}
	if strings.Count(content, "\n")+1 > c.limits.MaxLines {
		c.Error("file exceeds max_lines", 0, 0)
		return "", false
	}
	return content, true
}

// Resolve resolves an import relative to the current file; the resolver owns
// root confinement.
func (c *Context) Resolve(importPath string) (string, bool) {
	if importPath == "" || strings.ContainsRune(importPath, '\x00') {
		return "", false
	}
	if strings.HasPrefix(importPath, "/") {
		return "", false
	}
	if len(importPath) >= 3 && isASCIIAlpha(importPath[0]) && importPath[1] == ':' &&
		(importPath[2] == '\\' || importPath[2] == '/') {
		return "", false
	}
	dir := "."
	if index := strings.LastIndexByte(c.file, '/'); index >= 0 {
		dir = c.file[:index]
	}
	candidate, ok := normalizePath(dir + "/" + importPath)
	if !ok {
		return "", false
	}
	key, content, ok := c.resolver.Read(candidate)
	if !ok {
		if message := c.resolver.Failure(); message != "" {
			c.Error(message, 0, 0)
		}
		return "", false
	}
	c.contents[key] = content
	return key, true
}

func (c *Context) Push(realpath string) bool {
	if c.visited[realpath] {
		c.Warning("import cycle skipped", 0, 0)
		return false
	}
	if len(c.imports) >= c.limits.MaxImports {
		c.Error("max_imports exceeded", 0, 0)
		return false
	}
	if c.depth >= c.limits.MaxDepth {
		c.Error("max_depth exceeded", 0, 0)
		return false
	}
	c.visited[realpath] = true
	c.imports = append(c.imports, realpath)
	c.stack = append(c.stack, contextFrame{file: c.file, depth: c.depth})
	c.file = realpath
	c.depth++
	return true
}

func (c *Context) Pop() {
	if len(c.stack) == 0 {
		return
	}
	frame := c.stack[len(c.stack)-1]
	c.stack = c.stack[:len(c.stack)-1]
	c.file, c.depth = frame.file, frame.depth
}

func (c *Context) EnterFile(file string) {
	c.stack = append(c.stack, contextFrame{file: c.file, depth: c.depth})
	c.file = file
}

func (c *Context) LeaveFile() {
	c.Pop()
}

func (c *Context) Error(message string, line, column int64) {
	c.errors = append(c.errors, c.entry("error", message, line, column))
}

func (c *Context) Warning(message string, line, column int64) {
	c.warnings = append(c.warnings, c.entry("warning", message, line, column))
}

// Unsupported records unsupported syntax as an error in strict mode (the
// default) and a warning in lenient mode.
func (c *Context) Unsupported(message string, line, column int64) {
	if c.options.Lenient {
		c.Warning(message, line, column)
	} else {
		c.Error(message, line, column)
	}
}

func (c *Context) entry(severity, message string, line, column int64) Diagnostic {
	return Diagnostic{Path: "", Line: line, Column: column, Severity: severity, Message: message}
}

func (c *Context) Document(key string) *Node            { return c.documents[key] }
func (c *Context) RememberDocument(key string, n *Node) { c.documents[key] = n }
func (c *Context) ExpandedDocument(key string) *Node    { return c.expanded[key] }
func (c *Context) RememberExpanded(key string, n *Node) { c.expanded[key] = n }

func normalizePath(path string) (string, bool) {
	path = strings.ReplaceAll(path, "\\", "/")
	absolute := strings.HasPrefix(path, "/")
	var parts []string
	for _, segment := range strings.Split(path, "/") {
		switch segment {
		case "", ".":
		case "..":
			if len(parts) == 0 {
				return "", false
			}
			parts = parts[:len(parts)-1]
		default:
			parts = append(parts, segment)
		}
	}
	joined := strings.Join(parts, "/")
	if joined == "" {
		return "", false
	}
	if absolute {
		return "/" + joined, true
	}
	return joined, true
}

func isASCIIAlpha(ch byte) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z')
}

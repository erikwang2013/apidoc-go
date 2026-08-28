// Package parse turns annotated Go handler functions into apidoc
// registration records via go/ast, so docs live next to the code:
//
//	// @apidoc
//	// @method GET
//	// @url /api/v1/users/:id
//	// @title 获取用户
//	// @param id string query true "用户ID"
//	// @success ok User "成功"
//	func GetUser(c *gin.Context) (*User, error) { ... }
//
// ParseDir("./handlers") returns one Result per annotated function;
// register them with s.Register(apidoc.Route{Method: r.Method, URL: r.URL,
// Handler: GetUser, Doc: r.Doc}). A function without an @apidoc marker
// line is skipped; malformed tags return an error with the function's
// source position. Struct types declared in the same file are
// expanded into Param.Children / Response.Fields automatically.
package parse

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"

	"github.com/erikwang2013/apidoc-go/model"
)

// Result is one documented endpoint: Doc fills model.Doc (aliased as
// apidoc.Doc); Method/URL come from @method/@url, or stay empty for
// the caller to fill from the route.
type Result struct {
	Method, URL string
	Doc         model.Doc
}

// ParseFile parses one Go file and returns its annotated functions.
func ParseFile(path string) ([]Result, error) {
	return ParseSource(path, nil)
}

// ParseDir walks dir for .go files (skipping _test.go) and returns the
// annotated functions of all of them. Errors from individual files are
// aggregated; the walk continues past them.
func ParseDir(dir string) ([]Result, error) {
	var res []Result
	var errs []error
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			errs = append(errs, err)
			return nil
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rs, err := ParseFile(path)
		res = append(res, rs...)
		if err != nil {
			errs = append(errs, err)
		}
		return nil
	})
	if err != nil {
		errs = append(errs, err)
	}
	return res, errors.Join(errs...)
}

// ParseSource parses src (nil = read from path) and returns the
// annotated functions of the file.
func ParseSource(path string, src []byte) ([]Result, error) {
	fset := token.NewFileSet()
	// Pass src as a plain any: a typed-nil []byte would be non-nil to the
	// parser, which then treats it as an empty source instead of reading
	// the file from path.
	var srcAny any
	if src != nil {
		srcAny = src
	}
	f, err := parser.ParseFile(fset, path, srcAny, parser.ParseComments)
	if err != nil {
		return nil, err
	}
	var res []Result
	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Doc == nil || !annotated(fn.Doc) {
			continue
		}
		r, err := parseFunc(fset, f, fn)
		if err != nil {
			return res, err
		}
		res = append(res, r)
	}
	return res, nil
}

// annotated reports whether the comment group carries an @apidoc line.
func annotated(cg *ast.CommentGroup) bool {
	for _, c := range cg.List {
		f := strings.Fields(commentLine(c.Text))
		if len(f) > 0 && strings.TrimPrefix(f[0], "@") == "apidoc" {
			return true
		}
	}
	return false
}

func parseFunc(fset *token.FileSet, f *ast.File, fn *ast.FuncDecl) (Result, error) {
	var r Result
	if err := parseTags(fset, f, fn, &r); err != nil {
		return r, err
	}
	extractStructs(f, fn, &r)
	if r.Doc.Controller == "" {
		r.Doc.Controller = fn.Name.Name
	}
	if r.Doc.Title == "" {
		r.Doc.Title = fn.Name.Name
	}
	return r, nil
}

// parseTags applies one @tag per comment line.
func parseTags(fset *token.FileSet, f *ast.File, fn *ast.FuncDecl, r *Result) error {
	for _, c := range fn.Doc.List {
		flds := strings.Fields(commentLine(c.Text))
		if len(flds) == 0 || !strings.HasPrefix(flds[0], "@") {
			continue
		}
		kw := strings.TrimPrefix(flds[0], "@")
		toks := flds[1:]
		var err error
		switch kw {
		case "method":
			if len(toks) < 1 {
				err = fmt.Errorf("@method wants a method")
			} else {
				r.Method = toks[0]
			}
		case "url":
			r.URL = strings.Join(toks, " ")
		case "title":
			r.Doc.Title = strings.Join(toks, " ")
		case "desc":
			r.Doc.Desc = strings.Join(toks, " ")
		case "author":
			r.Doc.Author = strings.Join(toks, " ")
		case "app":
			r.Doc.App = strings.Join(toks, " ")
		case "version":
			r.Doc.Version = strings.Join(toks, " ")
		case "group":
			r.Doc.Controller = strings.Join(toks, " ")
		case "sort":
			if len(toks) < 1 {
				err = fmt.Errorf("@sort wants a number")
			} else if r.Doc.Sort, err = strconv.Atoi(toks[0]); err != nil {
				err = fmt.Errorf("bad @sort %q", toks[0])
			}
		case "param", "header", "body":
			var p model.Param
			if p, err = parseParam(f, kw, toks); err == nil {
				r.Doc.Params = append(r.Doc.Params, p)
			}
		case "success", "response":
			var resp model.Response
			if resp, err = parseResponse(toks); err == nil {
				r.Doc.Responses = append(r.Doc.Responses, resp)
			}
		case "markdown":
			if r.Doc.Markdown != "" {
				r.Doc.Markdown += "\n"
			}
			r.Doc.Markdown += strings.Join(toks, " ")
		}
		if err != nil {
			return fmt.Errorf("%s: %s: %v", fset.Position(fn.Pos()), fn.Name.Name, err)
		}
	}
	return nil
}

// parseParam parses "@param name type [true|false|1|0] [desc] [k=v...]".
// Desc runs up to the first k=v token; in=/default=/mock=/children=
// are supported (children expands a same-package struct one level).
func parseParam(f *ast.File, kw string, toks []string) (model.Param, error) {
	p := model.Param{In: "query"}
	if kw == "header" {
		p.In = "header"
	}
	if kw == "body" {
		p.In = "body"
	}
	if len(toks) < 2 {
		return p, fmt.Errorf("@%s wants name and type", kw)
	}
	p.Name, p.Type = toks[0], toks[1]
	i := 2
	if i < len(toks) {
		if b, err := strconv.ParseBool(toks[i]); err == nil {
			p.Required = b
			i++
		}
	}
	rest := toks[i:]
	j := indexKV(rest)
	p.Desc = strings.Join(rest[:j], " ")
	for _, kv := range rest[j:] {
		key, val, ok := strings.Cut(kv, "=")
		if !ok {
			return p, fmt.Errorf("bad k=v %q", kv)
		}
		switch key {
		case "in":
			switch val {
			case "header", "query", "body", "path":
				p.In = val
			default:
				return p, fmt.Errorf("bad in=%q", val)
			}
		case "default":
			p.Default = val
		case "mock":
			p.Mock = val
		case "children":
			p.Type = val
			if st := samePkgStructName(f, val); st != nil {
				p.Children = structFields(st)
			}
		default:
			return p, fmt.Errorf("unknown key %q", key)
		}
	}
	return p, nil
}

// parseResponse parses "@success [name] type [desc]". A leading token
// that looks like a type (builtin or capitalized) is the type, so the
// name is optional and defaults to "ok". A "{list}" prefix makes the
// type a slice.
func parseResponse(toks []string) (model.Response, error) {
	if len(toks) < 1 {
		return model.Response{}, fmt.Errorf("wants a type")
	}
	name, typeTok := "ok", 0
	if len(toks) >= 2 && toks[0] == "{list}" {
		toks[1] = "{list}" + toks[1]
		toks = toks[1:]
	}
	if len(toks) >= 2 && !isType(toks[0]) {
		name, typeTok = toks[0], 1
	}
	if len(toks) <= typeTok {
		return model.Response{}, fmt.Errorf("wants a type after %q", name)
	}
	resp := model.Response{Name: name, Type: toks[typeTok]}
	if strings.HasPrefix(resp.Type, "{list}") {
		resp.Type = "[]" + strings.TrimPrefix(resp.Type, "{list}")
	}
	for _, t := range toks[typeTok+1:] {
		if strings.Contains(t, "=") {
			return model.Response{}, fmt.Errorf("unknown key %q", t)
		}
	}
	if typeTok+1 < len(toks) {
		resp.Desc = strings.Join(toks[typeTok+1:], " ")
	}
	return resp, nil
}

// isType reports whether a token names a type: a Go builtin or any
// capitalized identifier (custom structs). Lowercase is a name.
func isType(tok string) bool {
	tok = strings.TrimPrefix(tok, "{list}")
	if builtins[tok] {
		return true
	}
	return tok != "" && tok[0] >= 'A' && tok[0] <= 'Z'
}

var builtins = map[string]bool{
	"string": true, "int": true, "int64": true, "float": true, "float64": true,
	"double": true, "bool": true, "boolean": true, "object": true, "array": true,
	"map": true, "json": true, "number": true, "file": true, "any": true,
}

// indexKV returns the index of the first k=v token.
func indexKV(toks []string) int {
	for i, t := range toks {
		if strings.Contains(t, "=") {
			return i
		}
	}
	return len(toks)
}

// extractStructs turns same-file struct handler params and results
// into a body Param with Children and a Response with Fields.
func extractStructs(f *ast.File, fn *ast.FuncDecl, r *Result) {
	if fn.Type.Params != nil {
		for _, p := range fn.Type.Params.List {
			if id, st := samePkgStruct(f, p.Type); st != nil {
				name := ""
				if len(p.Names) > 0 {
					name = p.Names[0].Name
				}
				r.Doc.Params = append(r.Doc.Params, model.Param{
					Name: name, In: "body", Type: id.Name, Children: structFields(st),
				})
			}
		}
	}
	if fn.Type.Results != nil {
		for _, p := range fn.Type.Results.List {
			if id, st := samePkgStruct(f, p.Type); st != nil {
				r.Doc.Responses = append(r.Doc.Responses, model.Response{
					Name: "ok", Type: id.Name, Fields: structFields(st),
				})
			}
		}
	}
}

// samePkgStruct resolves e (after unwrapping *T/[]T) to a struct type
// declared in the same package; returns the ident and the struct.
func samePkgStruct(f *ast.File, e ast.Expr) (*ast.Ident, *ast.StructType) {
	var id *ast.Ident
	for {
		switch t := e.(type) {
		case *ast.StarExpr:
			e = t.X
		case *ast.ArrayType:
			e = t.Elt
		default:
			id, _ = e.(*ast.Ident)
			e = nil
		}
		if e == nil {
			break
		}
	}
	if id == nil {
		return nil, nil
	}
	st := samePkgStructName(f, id.Name)
	if st == nil {
		return nil, nil
	}
	return id, st
}

func samePkgStructName(f *ast.File, name string) *ast.StructType {
	spec := f.Scope.Lookup(name)
	if spec == nil {
		return nil
	}
	ts, ok := spec.Decl.(*ast.TypeSpec)
	if !ok {
		return nil
	}
	st, _ := ts.Type.(*ast.StructType)
	return st
}

// structFields extracts one Param per exported field: name from the
// json tag's first segment (json:"-" and untagged unexported fields
// are skipped), type from the field expression.
func structFields(st *ast.StructType) []model.Param {
	var out []model.Param
	for _, fld := range st.Fields.List {
		for _, n := range fld.Names {
			name, skip := fieldName(n, fld.Tag)
			if skip {
				continue
			}
			out = append(out, model.Param{Name: name, Type: types.ExprString(fld.Type)})
		}
	}
	return out
}

func fieldName(n *ast.Ident, tag *ast.BasicLit) (string, bool) {
	if tag != nil {
		if j, ok := reflect.StructTag(strings.Trim(tag.Value, "`")).Lookup("json"); ok {
			name := strings.Split(j, ",")[0]
			if name == "-" {
				return "", true
			}
			if name != "" {
				return name, false
			}
		}
	}
	if !n.IsExported() {
		return "", true
	}
	return n.Name, false
}

// commentLine strips the comment markers from one comment text.
func commentLine(t string) string {
	t = strings.TrimSpace(t)
	if rest, ok := strings.CutPrefix(t, "//"); ok {
		return strings.TrimSpace(rest)
	}
	if rest, ok := strings.CutPrefix(t, "/*"); ok {
		return strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(rest), "*/"))
	}
	return ""
}

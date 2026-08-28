package apidoc

import (
	"reflect"
	"strings"
)

// reflectParams derives body params from a handler's struct args so docs
// work without hand-written params. Returns nil when no struct arg exists.
func reflectParams(h any) []Param {
	t := reflect.TypeOf(h)
	if t == nil || t.Kind() != reflect.Func {
		return nil
	}
	var out []Param
	for i := 0; i < t.NumIn(); i++ {
		if p := structParam(t.In(i)); p != nil {
			out = append(out, *p)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// deref unwraps *T/[]T to the underlying type.
func deref(t reflect.Type) reflect.Type {
	for t.Kind() == reflect.Ptr || t.Kind() == reflect.Slice {
		t = t.Elem()
	}
	return t
}

// structParam maps a struct arg to one body param.
func structParam(t reflect.Type) *Param {
	t = deref(t)
	if t.Kind() != reflect.Struct || isCtx(t) {
		return nil
	}
	name := t.Name()
	if name == "" { // unnamed struct: show its shape instead of an empty type
		name = t.String()
	}
	return &Param{In: "body", Type: name, Children: fields(t)}
}

// isCtx skips framework context args by their type name.
func isCtx(t reflect.Type) bool {
	switch t.Name() {
	case "Request", "ResponseWriter", "Context", "Ctx":
		return true
	}
	return false
}

// fields maps exported struct fields to params; json tags rename, "-"
// drops, struct-typed fields recurse into Children.
func fields(t reflect.Type) []Param {
	return fieldsSeen(t, map[reflect.Type]bool{})
}

func fieldsSeen(t reflect.Type, seen map[reflect.Type]bool) []Param {
	if seen[t] { // stops self-referential structs (Parent *Node) from recursing forever
		return nil
	}
	seen[t] = true
	defer delete(seen, t)
	var out []Param
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if f.PkgPath != "" {
			continue
		}
		name := f.Name
		if j := f.Tag.Get("json"); j != "" {
			seg := strings.Split(j, ",")[0]
			if seg == "-" {
				continue
			}
			if seg != "" {
				name = seg
			}
		}
		p := Param{Name: name, Type: f.Type.String()}
		if u := deref(f.Type); u.Kind() == reflect.Struct {
			p.Children = fieldsSeen(u, seen)
		}
		out = append(out, p)
	}
	return out
}

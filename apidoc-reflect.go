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

// structParam unwraps *T/[]T and maps a struct arg to one body param.
func structParam(t reflect.Type) *Param {
	for t.Kind() == reflect.Ptr || t.Kind() == reflect.Slice {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct || isCtx(t) {
		return nil
	}
	return &Param{In: "body", Type: t.Name(), Children: fields(t)}
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
	var out []Param
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if f.PkgPath != "" { // unexported
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
		u := f.Type
		for u.Kind() == reflect.Ptr || u.Kind() == reflect.Slice {
			u = u.Elem()
		}
		if u.Kind() == reflect.Struct {
			p.Children = fields(u)
		}
		out = append(out, p)
	}
	return out
}

// Package mock generates deterministic example values for documented
// types: field-level mocks for the detail endpoint and any future
// exporter. Stdlib only.
package mock

import (
	"encoding/json"
	"strings"

	"github.com/erikwang2013/apidoc-go/model"
)

// Value returns a deterministic example value for a documented type:
// strings get "sample", ints 0, floats 0.0, bools true, []T a
// one-element array, objects (children) a map of child mocks keyed by
// name, and anything unknown null.
func Value(t string, children []model.Param) any {
	t = strings.TrimPrefix(t, "{list}")
	if strings.HasPrefix(t, "[]") {
		return []any{Value(t[2:], children)}
	}
	switch t {
	case "string":
		return "sample"
	case "int", "int64", "number":
		return 0
	case "float", "float64", "double":
		return 0.0
	case "bool", "boolean":
		return true
	case "object", "struct":
		return object(children)
	}
	if len(children) > 0 {
		return object(children)
	}
	return nil
}

// Action returns the JSON of one example value per param, keyed by
// param name. The detail endpoint serves it as the action's mock when
// none was registered.
func Action(a *model.Action) string {
	m := make(map[string]any, len(a.Params))
	for _, p := range a.Params {
		m[p.Name] = Value(p.Type, p.Children)
	}
	b, err := json.Marshal(m)
	if err != nil {
		return "{}"
	}
	return string(b)
}

func object(children []model.Param) map[string]any {
	out := make(map[string]any, len(children))
	for _, c := range children {
		out[c.Name] = Value(c.Type, c.Children)
	}
	return out
}

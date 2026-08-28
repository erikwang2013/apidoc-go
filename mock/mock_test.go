package mock

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/erikwang2013/apidoc-go/model"
)

func TestValueTable(t *testing.T) {
	cases := []struct {
		name     string
		typ      string
		children []model.Param
		want     any
	}{
		{"string", "string", nil, "sample"},
		{"int", "int", nil, 0},
		{"int64", "int64", nil, 0},
		{"number", "number", nil, 0},
		{"float", "float", nil, 0.0},
		{"double", "double", nil, 0.0},
		{"bool", "bool", nil, true},
		{"boolean", "boolean", nil, true},
		{"array of int", "[]int", nil, []any{0}},
		{"array of string", "[]string", nil, []any{"sample"}},
		{"list-wrapped", "{list}string", nil, "sample"},
		{"array list-wrapped", "[]{list}string", nil, []any{"sample"}},
		{"object children", "object", []model.Param{
			{Name: "id", Type: "int"},
			{Name: "name", Type: "string"},
			{Name: "tags", Type: "[]bool"},
		}, map[string]any{"id": 0, "name": "sample", "tags": []any{true}}},
		{"struct children", "struct", []model.Param{{Name: "x", Type: "float"}}, map[string]any{"x": 0.0}},
		{"unknown", "???", nil, nil},
		{"unknown with children is object", "Custom", []model.Param{{Name: "a", Type: "string"}}, map[string]any{"a": "sample"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Value(c.typ, c.children)
			if !reflect.DeepEqual(got, c.want) {
				t.Fatalf("Value(%q)=%#v, want %#v", c.typ, got, c.want)
			}
		})
	}
}

func TestValueNilChildrenNoPanic(t *testing.T) {
	for _, typ := range []string{"object", "struct", "[]object", "", "string"} {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("Value(%q, nil) panicked: %v", typ, r)
				}
			}()
			_ = Value(typ, nil)
		}()
	}
	// deep nesting: child objects with nil children
	got := Value("object", []model.Param{{Name: "sub", Type: "object"}})
	want := map[string]any{"sub": map[string]any{}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("nested nil-children object = %#v, want %#v", got, want)
	}
}

func TestAction(t *testing.T) {
	a := &model.Action{App: "users", Version: "v1",
		Params: []model.Param{
			{Name: "id", In: "query", Type: "int"},
			{Name: "name", In: "query", Type: "string"},
			{Name: "user", In: "query", Type: "object", Children: []model.Param{{Name: "id", Type: "int"}}},
		}}
	got := Action(a)
	var m map[string]any
	if err := json.Unmarshal([]byte(got), &m); err != nil {
		t.Fatalf("Action returned invalid JSON %q: %v", got, err)
	}
	// JSON numbers decode as float64.
	want := map[string]any{"id": float64(0), "name": "sample", "user": map[string]any{"id": float64(0)}}
	if !reflect.DeepEqual(m, want) {
		t.Fatalf("Action() = %v, want %v", m, want)
	}
}

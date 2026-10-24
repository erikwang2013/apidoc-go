// Package xjson bridges the JSON bodies used by the debug UI and the generic
// values the Hessian2 codec understands (int64 instead of json.Number, and
// JSON-safe maps on the way back).
package xjson

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
)

// Args parses a body that must be a JSON array of positional RPC arguments.
// An empty body means no arguments.
func Args(body []byte) ([]any, error) {
	body = bytes.TrimSpace(body)
	if len(body) == 0 {
		return nil, nil
	}
	var raw []json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("body must be a JSON array of arguments: %w", err)
	}
	out := make([]any, len(raw))
	for i, r := range raw {
		dec := json.NewDecoder(bytes.NewReader(r))
		dec.UseNumber()
		var v any
		if err := dec.Decode(&v); err != nil {
			return nil, fmt.Errorf("argument %d: %w", i, err)
		}
		out[i] = normalize(v)
	}
	return out, nil
}

func normalize(v any) any {
	switch x := v.(type) {
	case json.Number:
		if i, err := x.Int64(); err == nil {
			return i
		}
		if f, err := x.Float64(); err == nil {
			return f
		}
		return x.String()
	case []any:
		for i := range x {
			x[i] = normalize(x[i])
		}
	case map[string]any:
		for k := range x {
			x[k] = normalize(x[k])
		}
	}
	return v
}

// Value converts a decoded Hessian value into something encoding/json can
// marshal: map keys become strings and byte slices become base64 strings.
func Value(v any) any {
	switch x := v.(type) {
	case map[any]any:
		m := make(map[string]any, len(x))
		for k, val := range x {
			m[fmt.Sprint(k)] = Value(val)
		}
		return m
	case map[string]any:
		m := make(map[string]any, len(x))
		for k, val := range x {
			m[k] = Value(val)
		}
		return m
	case []any:
		s := make([]any, len(x))
		for i := range x {
			s[i] = Value(x[i])
		}
		return s
	case []byte:
		return base64.StdEncoding.EncodeToString(x)
	}
	return v
}

package protocol

import (
	"strings"
)

// Parsed is one adapter's normalized result before validation aggregation.
type Parsed struct {
	Operation string
	Messages  []map[string]any
	Meta      map[string]any
}

func emptyParsed() Parsed {
	return Parsed{Messages: []map[string]any{}, Meta: map[string]any{}}
}

func newMessage(name, direction string, fields []map[string]any) map[string]any {
	if fields == nil {
		fields = []map[string]any{}
	}
	return map[string]any{"name": name, "direction": direction, "fields": fields}
}

func refsFor(document *Node, loader Loader) *RefCtx {
	refs := NewRefCtx(document, "", loader)
	return &refs
}

// sampleValidationFor validates a JSON sample. With a schema it uses the schema
// validator; otherwise required top-level fields are checked.
func sampleValidationFor(sample string, schema *Node, fields []map[string]any, refs *RefCtx, ctx *Context) SampleValidation {
	data, message := ParseJSON(sample)
	if message != "" {
		return SampleValidation{
			OK: false,
			Errors: []SampleError{{
				Path:     "$",
				Severity: "error",
				Message:  "JSON sample parse error: " + message,
			}},
			Warnings: []SampleError{},
		}
	}
	if schema != nil {
		failures := ValidateSchema(schema, data, refs, ctx, "$", 0)
		errors := make([]SampleError, len(failures))
		for i, failure := range failures {
			errors[i] = SampleError{Path: failure.Path, Severity: "error", Message: failure.Message}
		}
		return SampleValidation{OK: len(errors) == 0, Errors: errors, Warnings: []SampleError{}}
	}
	errors := []SampleError{}
	if data.IsArrayLike() {
		for _, field := range fields {
			require, _ := field["require"].(bool)
			name, _ := field["name"].(string)
			if require && data.Get(name) == nil {
				errors = append(errors, SampleError{
					Path:     "$." + name,
					Severity: "error",
					Message:  "required field missing",
				})
			}
		}
	}
	return SampleValidation{OK: len(errors) == 0, Errors: errors, Warnings: []SampleError{}}
}

func sampleValidationFromFailures(failures []schemaFailure) SampleValidation {
	errors := make([]SampleError, len(failures))
	for i, failure := range failures {
		errors[i] = SampleError{Path: failure.Path, Severity: "error", Message: failure.Message}
	}
	return SampleValidation{OK: len(errors) == 0, Errors: errors, Warnings: []SampleError{}}
}

func trimDocStart(content string) string {
	return strings.TrimLeft(content, " \t\r\n\f\v")
}

package protocol

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const fixtureRoot = "testdata"

// fixtureResolver serves fixture files under testdata and refuses paths that
// escape it. Keys are the requested relative paths, matching Context.Resolve.
type fixtureResolver struct {
	failure string
	reads   []string
}

func (r *fixtureResolver) Read(path string) (string, string, bool) {
	clean := filepath.Clean(path)
	if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", "", false
	}
	r.reads = append(r.reads, clean)
	content, err := os.ReadFile(filepath.Join(fixtureRoot, clean))
	if err != nil {
		return "", "", false
	}
	return clean, string(content), true
}

func (r *fixtureResolver) Failure() string { return r.failure }

func readFixture(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(fixtureRoot, path))
	if err != nil {
		t.Fatalf("read fixture %s: %v", path, err)
	}
	return string(content)
}

func TestSharedFixturesMatchPHPReference(t *testing.T) {
	var cases []map[string]any
	decodeJSONFile(t, "parity/cases.json", &cases)
	var expected map[string]map[string]any
	decodeJSONFile(t, "parity/php.json", &expected)

	for _, c := range cases {
		name := c["name"].(string)
		format := c["format"].(string)
		file := c["file"].(string)
		content, hasContent := c["content"].(string)
		if !hasContent {
			content = readFixture(t, strings.TrimPrefix(file, "tests/fixtures/protocol/"))
		}
		options := Options{}
		if value, ok := c["operation"].(string); ok {
			options.Operation = value
		}
		if value, ok := c["message"].(string); ok {
			options.Message = value
		}
		if value, ok := c["direction"].(string); ok {
			options.Direction = value
		}
		if value, ok := c["sample"].(string); ok {
			options.Sample = value
		}
		if value, ok := c["strict"].(bool); ok {
			options.Lenient = !value
		}
		source := strings.TrimPrefix(file, "tests/fixtures/protocol/")
		resolver := &fixtureResolver{}
		definition := Parse(format, source, content, options, DefaultLimits(), resolver)

		actual := toGeneric(t, map[string]any{
			"operation": definition.Operation,
			"messages":  definition.Messages,
			"meta":      definition.Meta,
			"errors":    definition.Validation.Errors,
			"warnings":  definition.Validation.Warnings,
		})
		want := expected[name]
		wantMeta := want["meta"]
		if list, ok := wantMeta.([]any); ok && len(list) == 0 {
			wantMeta = map[string]any{}
		}
		want = map[string]any{
			"operation": want["operation"],
			"messages":  want["messages"],
			"meta":      wantMeta,
			"errors":    want["errors"],
			"warnings":  want["warnings"],
		}
		if !reflect.DeepEqual(actual, want) {
			actualJSON, _ := json.Marshal(actual)
			wantJSON, _ := json.Marshal(want)
			t.Errorf("%s mismatch\nactual: %s\nwant:   %s", name, actualJSON, wantJSON)
		}
	}
}

func decodeJSONFile(t *testing.T, path string, target any) {
	t.Helper()
	if err := json.Unmarshal([]byte(readFixture(t, path)), target); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
}

func toGeneric(t *testing.T, value any) any {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var result any
	if err := json.Unmarshal(encoded, &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return result
}

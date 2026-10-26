package uiapi

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/erikwang2013/apidoc-go/model"
	"github.com/erikwang2013/apidoc-go/parse/protocol"
	"github.com/erikwang2013/apidoc-go/store"
)

func TestDefinitionResolverConfinesAndCaps(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "user.json"), []byte(`{"type":"object"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	resolver := newDefinitionResolver(root)
	key, content, ok := resolver.Read("user.json")
	if !ok || content != `{"type":"object"}` || key != "user.json" {
		t.Fatalf("read: %q %q %v", key, content, ok)
	}
	// Keys stay root-relative so nested imports resolve beside the declaration.
	if key, _, ok := resolver.Read("./sub/../user.json"); ok || key != "" {
		t.Fatalf("traversal-like key accepted: %q %v", key, ok)
	}
	key, _, ok = resolver.Read("./user.json")
	if !ok || key != "user.json" {
		t.Fatalf("normalized key: %q %v", key, ok)
	}

	for _, path := range []string{"../outside.json", "/etc/passwd", "dir/../user.json", "user.json\x00"} {
		if _, _, ok := resolver.Read(path); ok {
			t.Fatalf("%q must be rejected", path)
		}
	}
	if _, _, ok := resolver.Read("missing.json"); ok || resolver.Failure() != "" {
		t.Fatalf("missing file: ok=%v failure=%q", ok, resolver.Failure())
	}
	if err := os.Mkdir(filepath.Join(root, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, _, ok := resolver.Read("sub"); ok {
		t.Fatal("directories must be rejected")
	}

	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.json"), []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "link")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, _, ok := resolver.Read("link/secret.json"); ok {
		t.Fatal("symlink escape must be rejected")
	}

	limits := protocol.DefaultLimits()
	if err := os.WriteFile(filepath.Join(root, "big.json"), []byte(strings.Repeat("x", limits.MaxFileBytes+1)), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, ok := resolver.Read("big.json"); ok || resolver.Failure() != "file exceeds max_file_bytes" {
		t.Fatalf("oversized: ok=%v failure=%q", ok, resolver.Failure())
	}
	if err := os.WriteFile(filepath.Join(root, "lines.json"), []byte(strings.Repeat("x\n", limits.MaxLines+1)), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, ok := resolver.Read("lines.json"); ok || resolver.Failure() != "file exceeds max_lines" {
		t.Fatalf("line cap: ok=%v failure=%q", ok, resolver.Failure())
	}
}

func TestDetailResolvesNestedRelativeRefs(t *testing.T) {
	st := store.New("Demo")
	st.SetAppMeta("default", "Default")
	st.Register("POST", "/users", model.Doc{
		App: "default", Version: "v1", Controller: "users", Title: "Create",
		Definitions: []model.Definition{{File: "openapi/api.yaml", Format: "openapi", Operation: "createUser"}},
	})
	h := newHandlerWith(t, Options{
		Store:          st,
		DefinitionRoot: filepath.Join("..", "..", "parse", "protocol", "testdata"),
	})

	body := `{"appKey":"default|v1","path":"` + store.ActionID("default", "v1", "POST", "/users") + `"}`
	detail := decodeData(t, do(t, h, "POST", "/ui/v1/detail", body, ""))
	if validation := detail["validation"].(map[string]any); validation["ok"] != true {
		t.Fatalf("validation: %#v", validation)
	}
	definitions := detail["definitions"].([]any)
	definition := definitions[0].(map[string]any)
	if definition["operation"] != "createUser" {
		t.Fatalf("definition: %#v", definition)
	}
	request := definition["messages"].([]any)[0].(map[string]any)
	fields := request["fields"].([]any)
	if len(fields) != 2 {
		t.Fatalf("request fields: %#v", fields)
	}
	id := fields[0].(map[string]any)
	if id["name"] != "id" || id["type"] != "integer" || id["require"] != true {
		t.Fatalf("nested $ref field: %#v", id)
	}
}

func TestDetailReturnsParsedDefinitions(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "user.json"),
		[]byte(`{"title":"User","type":"object","properties":{"id":{"type":"integer"}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	st := store.New("Demo")
	st.SetAppMeta("default", "Default")
	st.Register("POST", "/users", model.Doc{
		App: "default", Version: "v1", Controller: "users", Title: "Create",
		Definitions: []model.Definition{{File: "user.json", Format: "json-schema"}},
	})
	st.Register("POST", "/broken", model.Doc{
		App: "default", Version: "v1", Controller: "users", Title: "Broken",
		Definitions: []model.Definition{{File: "missing.json", Format: "json-schema"}},
	})
	h := newHandlerWith(t, Options{Store: st, DefinitionRoot: root})

	body := `{"appKey":"default|v1","path":"` + store.ActionID("default", "v1", "POST", "/users") + `"}`
	detail := decodeData(t, do(t, h, "POST", "/ui/v1/detail", body, ""))
	definitions, _ := detail["definitions"].([]any)
	if len(definitions) != 1 {
		t.Fatalf("definitions: %#v", detail["definitions"])
	}
	definition := definitions[0].(map[string]any)
	if definition["format"] != "json-schema" || definition["source"] != "user.json" {
		t.Fatalf("definition: %#v", definition)
	}
	if messages := definition["messages"].([]any); messages[0].(map[string]any)["name"] != "User" {
		t.Fatalf("messages: %#v", messages)
	}
	if validation := detail["validation"].(map[string]any); validation["ok"] != true {
		t.Fatalf("validation: %#v", validation)
	}

	body = `{"appKey":"default|v1","path":"` + store.ActionID("default", "v1", "POST", "/broken") + `"}`
	detail = decodeData(t, do(t, h, "POST", "/ui/v1/detail", body, ""))
	validation := detail["validation"].(map[string]any)
	errors := validation["errors"].([]any)
	if validation["ok"] != false || len(errors) != 1 ||
		errors[0].(map[string]any)["message"] != "definition file not found or outside allowed roots" {
		t.Fatalf("validation: %#v", validation)
	}
}

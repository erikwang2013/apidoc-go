package server

import (
	"strings"
	"testing"
)

func TestDebugSnippetsAndFileUI(t *testing.T) {
	js := string(debugJSPayload)
	for _, want := range []string{
		"dg-snippets", "createElement('details')", "{{method}}", "{{url}}", "{{body}}",
		"navigator.clipboard", "execCommand", "fallbackCopy",
		"el.type = 'file'", "FileReader", "maxDebugFile = 256 * 1024",
		"file.size > maxDebugFile", "dataset.file", "dataset.b64",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("debug payload missing %q", want)
		}
	}
	if strings.Contains(js, "innerHTML") {
		t.Fatal("debug payload must not use innerHTML")
	}
	if strings.Contains(js, "localStorage") {
		t.Fatal("debug payload must not persist values")
	}
	if !strings.Contains(string(indexHTML), `id="dg-snippets"`) {
		t.Error("index.html missing the #dg-snippets container")
	}
	if !strings.Contains(js, "Code snippets") {
		t.Error("debug payload missing the snippets summary")
	}
}

package uiapi

import (
	"io"
	"os"
	pathpkg "path"
	"path/filepath"
	"strings"

	"github.com/erikwang2013/apidoc-go/parse/protocol"
)

// definitionResolver is the production Resolver for @definition files: it only
// reads regular files inside DefinitionRoot and applies the parser's limits.
type definitionResolver struct {
	root    string
	failure string
}

func newDefinitionResolver(root string) *definitionResolver {
	return &definitionResolver{root: root}
}

func (r *definitionResolver) Failure() string { return r.failure }

func (r *definitionResolver) Read(path string) (string, string, bool) {
	r.failure = ""
	if path == "" || strings.ContainsRune(path, '\x00') || filepath.IsAbs(path) {
		return "", "", false
	}
	slashed := strings.ReplaceAll(path, "\\", "/")
	if len(slashed) >= 2 && slashed[1] == ':' {
		return "", "", false
	}
	for _, segment := range strings.Split(slashed, "/") {
		if segment == ".." {
			return "", "", false
		}
	}
	key := pathpkg.Clean(slashed)
	root := r.root
	if root == "" {
		var err error
		if root, err = os.Getwd(); err != nil {
			r.failure = "definition root unavailable"
			return "", "", false
		}
	}
	root, err := filepath.Abs(root)
	if err != nil {
		r.failure = "definition root unavailable"
		return "", "", false
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		r.failure = "definition root unavailable"
		return "", "", false
	}
	resolved, err := filepath.EvalSymlinks(filepath.Join(root, filepath.FromSlash(key)))
	if err != nil {
		return "", "", false // missing: the parser reports not-found
	}
	if !withinDefinitionRoot(root, resolved) {
		return "", "", false
	}
	info, err := os.Stat(resolved)
	if err != nil {
		r.failure = "unable to stat file"
		return "", "", false
	}
	if !info.Mode().IsRegular() {
		return "", "", false
	}
	limits := protocol.DefaultLimits()
	if info.Size() > int64(limits.MaxFileBytes) {
		r.failure = "file exceeds max_file_bytes"
		return "", "", false
	}
	file, err := os.Open(resolved)
	if err != nil {
		r.failure = "unable to read file"
		return "", "", false
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, int64(limits.MaxFileBytes)+1))
	if err != nil {
		r.failure = "unable to read file"
		return "", "", false
	}
	if len(data) > limits.MaxFileBytes {
		r.failure = "file exceeds max_file_bytes"
		return "", "", false
	}
	if strings.Count(string(data), "\n")+1 > limits.MaxLines {
		r.failure = "file exceeds max_lines"
		return "", "", false
	}
	// The key is the requested root-relative path, not the canonical one: the
	// parser resolves nested imports relative to it.
	return key, string(data), true
}

// withinDefinitionRoot compares canonical paths with a separator boundary.
func withinDefinitionRoot(root, target string) bool {
	rel, err := filepath.Rel(root, target)
	if err != nil || filepath.IsAbs(rel) {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

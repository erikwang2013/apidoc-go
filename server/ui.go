package server

import (
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// ui serves a packaged UIC-1 Vue build under the mount root. The root is
// opened once as an os.Root so traversal and symlinks escaping UIPath fail
// closed; directories are never listed.
type ui struct {
	root *os.Root
	err  error // UIPath invalid: every UI request fails closed with this
}

// newUI opens uiPath and validates index.html. server.Handler only calls it
// when UIPath is non-empty; the library default leaves the bundled UI off.
func newUI(uiPath string) *ui {
	abs, err := filepath.Abs(uiPath)
	var root *os.Root
	if err == nil {
		if root, err = os.OpenRoot(abs); err == nil {
			var st fs.FileInfo
			if st, err = root.Stat("index.html"); err == nil && !st.Mode().IsRegular() {
				err = errors.New("index.html is not a regular file")
			}
			if err == nil {
				return &ui{root: root}
			}
			root.Close()
		}
	}
	return &ui{err: fmt.Errorf("apidoc: UI path %q: %w", uiPath, err)}
}

func (u *ui) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if u.err != nil {
		http.Error(w, u.err.Error(), http.StatusInternalServerError)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	// Clean first so ".." and duplicate slashes cannot escape; os.Root checks
	// the result again before touching the filesystem.
	name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
	// API namespaces never fall back to the SPA: unknown API paths are 404.
	if name == "api" || strings.HasPrefix(name, "api/") ||
		name == "ui" || strings.HasPrefix(name, "ui/") {
		http.NotFound(w, r)
		return
	}
	if name == "" {
		u.serveIndex(w, r)
		return
	}
	f, err := u.root.Open(name)
	if err == nil {
		st, statErr := f.Stat()
		regular := statErr == nil && st.Mode().IsRegular()
		if regular {
			defer f.Close()
			http.ServeContent(w, r, name, st.ModTime(), f)
			return
		}
		f.Close()
		if statErr != nil {
			http.Error(w, "apidoc: UI file unavailable", http.StatusInternalServerError)
			return
		}
		// Directory or special file: no listing, no SPA fallback.
		http.NotFound(w, r)
		return
	}
	if !errors.Is(err, fs.ErrNotExist) {
		http.Error(w, "apidoc: UI file unavailable", http.StatusInternalServerError)
		return
	}
	// Unknown extensionless paths are SPA routes; missing assets stay 404.
	if path.Ext(name) == "" {
		u.serveIndex(w, r)
		return
	}
	http.NotFound(w, r)
}

// serveIndex fails closed if index.html disappeared after startup validation.
func (u *ui) serveIndex(w http.ResponseWriter, r *http.Request) {
	f, err := u.root.Open("index.html")
	if err != nil {
		http.Error(w, "apidoc: UI index.html unavailable", http.StatusInternalServerError)
		return
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil || !st.Mode().IsRegular() {
		http.Error(w, "apidoc: UI index.html unavailable", http.StatusInternalServerError)
		return
	}
	http.ServeContent(w, r, "index.html", st.ModTime(), f)
}

// prefixEntry redirects the bare mount prefix (which adapters strip to an
// empty path) to {prefix}/ so the UI's relative asset URLs resolve. Without
// it ServeMux would redirect to the host root.
type prefixEntry struct {
	next   http.Handler
	prefix string
}

func (h prefixEntry) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "" {
		h.next.ServeHTTP(w, r)
		return
	}
	loc := h.prefix + "/"
	if r.URL.RawQuery != "" {
		loc += "?" + r.URL.RawQuery
	}
	http.Redirect(w, r, loc, http.StatusTemporaryRedirect)
}

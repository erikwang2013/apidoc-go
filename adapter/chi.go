package adapter

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

// Chi wraps a *chi.Mux.
type Chi struct{ mux *chi.Mux }

// NewChi returns an adapter for the given mux.
func NewChi(mux *chi.Mux) *Chi { return &Chi{mux: mux} }

// Register implements Framework.
func (a *Chi) Register(method, path string, h any) (err error) {
	hf, err := handler[http.HandlerFunc](h)
	if err != nil {
		return err
	}
	defer recoverRegister("chi", method, path, &err)
	a.mux.Method(method, path, hf)
	return nil
}

// Mount implements Framework. chi's Mount registers both the exact
// pattern and its subtree itself, so a single call covers {prefix} and
// {prefix}/anything; a second Mount(prefix+"/", ...) would panic on a
// duplicate route.
func (a *Chi) Mount(prefix string, h http.Handler) {
	a.mux.Mount(prefix, http.StripPrefix(prefix, h))
}

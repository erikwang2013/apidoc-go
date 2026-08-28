package adapter

import (
	"fmt"
	"net/http"
)

// NetHTTP wraps a *http.ServeMux (Go 1.22+ method patterns).
type NetHTTP struct{ mux *http.ServeMux }

// NewNetHTTP returns an adapter for the given mux.
func NewNetHTTP(mux *http.ServeMux) *NetHTTP { return &NetHTTP{mux: mux} }

// Register implements Framework.
func (a *NetHTTP) Register(method, path string, h any) (err error) {
	hf, ok := h.(http.HandlerFunc)
	if !ok {
		return fmt.Errorf("apidoc: expected http.HandlerFunc, got %T", h)
	}
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("apidoc: net/http register %s %s: %v", method, path, p)
		}
	}()
	a.mux.Handle(method+" "+path, hf)
	return nil
}

// Mount implements Framework.
func (a *NetHTTP) Mount(prefix string, h http.Handler) {
	a.mux.Handle(prefix, http.StripPrefix(prefix, h))
	a.mux.Handle(prefix+"/", http.StripPrefix(prefix, h))
}

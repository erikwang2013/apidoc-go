package adapter

import "net/http"

// NetHTTP wraps a *http.ServeMux (Go 1.22+ method patterns).
type NetHTTP struct{ mux *http.ServeMux }

// NewNetHTTP returns an adapter for the given mux.
func NewNetHTTP(mux *http.ServeMux) *NetHTTP { return &NetHTTP{mux: mux} }

// Register implements Framework.
func (a *NetHTTP) Register(method, path string, h any) (err error) {
	hf, err := handler[http.HandlerFunc](h)
	if err != nil {
		return err
	}
	defer recoverRegister("net/http", method, path, &err)
	a.mux.Handle(method+" "+path, hf)
	return nil
}

// Mount implements Framework.
func (a *NetHTTP) Mount(prefix string, h http.Handler) {
	a.mux.Handle(prefix, http.StripPrefix(prefix, h))
	a.mux.Handle(prefix+"/", http.StripPrefix(prefix, h))
}

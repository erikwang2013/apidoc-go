package adapter

import (
	"net/http"

	khttp "github.com/go-kratos/kratos/v2/transport/http"
)

// Kratos wraps a *khttp.Server.
type Kratos struct {
	srv *khttp.Server
	r   *khttp.Router
}

// NewKratos returns an adapter for the given server.
func NewKratos(srv *khttp.Server) *Kratos {
	return &Kratos{srv: srv, r: srv.Route("")}
}

// Register implements Framework.
func (a *Kratos) Register(method, path string, h any) (err error) {
	hf, err := handler[khttp.HandlerFunc](h)
	if err != nil {
		return err
	}
	defer recoverRegister("kratos", method, path, &err)
	a.r.Handle(method, path, hf)
	return nil
}

// Mount implements Framework.
func (a *Kratos) Mount(prefix string, h http.Handler) {
	sh := http.StripPrefix(prefix, h)
	// HandlePrefix(prefix+"/") matches {prefix}/... via a literal regexp
	// that never matches the bare {prefix}, so the exact route covers it;
	// the trailing slash keeps sibling paths like /apidocFoo out.
	// The prefix route must come first: gorilla matches in registration
	// order, and the exact route's StrictSlash would otherwise 301
	// {prefix}/ away to {prefix} before the prefix route can serve it.
	a.srv.HandlePrefix(prefix+"/", sh)
	a.srv.Handle(prefix, sh)
}

package adapter

import (
	"net/http"

	"github.com/zeromicro/go-zero/rest"
)

// GoZero wraps a *rest.Server.
type GoZero struct{ srv *rest.Server }

// NewGoZero returns an adapter for the given server.
func NewGoZero(srv *rest.Server) *GoZero { return &GoZero{srv: srv} }

// Register implements Framework. rest.Route takes one http.HandlerFunc per
// method+path; an invalid method or duplicate route is rejected, not at
// AddRoute time but when the server starts and binds its routes.
func (a *GoZero) Register(method, path string, h any) (err error) {
	hf, err := handler[http.HandlerFunc](h)
	if err != nil {
		return err
	}
	defer recoverRegister("go-zero", method, path, &err)
	a.srv.AddRoute(rest.Route{Method: method, Path: path, Handler: hf})
	return nil
}

// Mount implements Framework. go-zero's router has no catch-all — only
// single-segment :params — so the doc server's route surface is
// registered explicitly, under every method it answers: GET for the UI
// and JSON API, POST for the login endpoints, OPTIONS so CORS preflights
// reach the doc server's middleware. The router path.Cleans the
// registered route and the request path alike, so the bare {prefix}
// entry serves {prefix}/ too; adding a literal {prefix}/ route would be
// the same cleaned route, which go-zero rejects as a duplicate at Start.
func (a *GoZero) Mount(prefix string, h http.Handler) {
	sh := http.HandlerFunc(http.StripPrefix(prefix, h).ServeHTTP)
	for _, path := range []string{
		prefix,
		prefix + "/api/menus",
		prefix + "/api/detail",
		prefix + "/api/export",
		prefix + "/api/login",
		prefix + "/api/app-login",
		prefix + "/apidoc-pet.svg",
	} {
		for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodOptions} {
			a.srv.AddRoute(rest.Route{Method: method, Path: path, Handler: sh})
		}
	}
}

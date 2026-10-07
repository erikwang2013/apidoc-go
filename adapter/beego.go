package adapter

import (
	"net/http"

	"github.com/beego/beego/v2/server/web"
)

// Beego wraps a *web.HttpServer (note beego's spelling: web.NewHttpSever).
type Beego struct{ app *web.HttpServer }

// NewBeego returns an adapter for the given server.
func NewBeego(app *web.HttpServer) *Beego { return &Beego{app: app} }

// Register implements Framework. beego's Handler registers the handler for
// every HTTP method of the pattern, so h is wrapped in a method guard that
// answers other methods with 405. beego keys routes by pattern, not by
// method: a second registration for the same path is prepended and shadows
// the first (last registration wins), so register one method per path.
func (a *Beego) Register(method, path string, h any) (err error) {
	hf, err := handler[http.HandlerFunc](h)
	if err != nil {
		return err
	}
	defer recoverRegister("beego", method, path, &err)
	a.app.Handler(path, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != method {
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
			return
		}
		hf(w, r)
	}))
	return nil
}

// Mount implements Framework. The bool option makes beego register
// path.Join(prefix, "?:all(.*)") for every method, which matches both
// {prefix} and {prefix}/anything, so one call covers the whole subtree.
func (a *Beego) Mount(prefix string, h http.Handler) {
	a.app.Handler(prefix, http.StripPrefix(prefix, h), true)
}

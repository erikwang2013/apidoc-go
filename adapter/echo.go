package adapter

import (
	"net/http"

	"github.com/labstack/echo/v4"
)

// Echo wraps an *echo.Echo.
type Echo struct{ e *echo.Echo }

// NewEcho returns an adapter for the given echo instance.
func NewEcho(e *echo.Echo) *Echo { return &Echo{e: e} }

// Register implements Framework.
func (a *Echo) Register(method, path string, h any) (err error) {
	hf, err := handler[echo.HandlerFunc](h)
	if err != nil {
		return err
	}
	defer recoverRegister("echo", method, path, &err)
	a.e.Add(method, path, hf)
	return nil
}

// Mount implements Framework.
func (a *Echo) Mount(prefix string, h http.Handler) {
	h = http.StripPrefix(prefix, h)
	a.e.Any(prefix, echo.WrapHandler(h))
	a.e.Any(prefix+"/*", echo.WrapHandler(h))
}

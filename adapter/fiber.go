package adapter

import (
	"fmt"
	"net/http"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/adaptor"
)

// Fiber wraps a *fiber.App (v2).
type Fiber struct{ app *fiber.App }

// NewFiber returns an adapter for the given app.
func NewFiber(app *fiber.App) *Fiber { return &Fiber{app: app} }

// Register implements Framework.
func (a *Fiber) Register(method, path string, h any) (err error) {
	hf, ok := h.(fiber.Handler)
	if !ok {
		return fmt.Errorf("apidoc: expected fiber.Handler, got %T", h)
	}
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("apidoc: fiber register %s %s: %v", method, path, p)
		}
	}()
	a.app.Add(method, path, hf)
	return nil
}

// Mount implements Framework.
func (a *Fiber) Mount(prefix string, h http.Handler) {
	hf := adaptor.HTTPHandler(http.StripPrefix(prefix, h))
	a.app.All(prefix, hf)
	a.app.All(prefix+"/*", hf)
}

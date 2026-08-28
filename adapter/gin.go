package adapter

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
)

// Gin wraps a *gin.Engine.
type Gin struct{ e *gin.Engine }

// NewGin returns an adapter for the given engine.
func NewGin(e *gin.Engine) *Gin { return &Gin{e: e} }

// Register implements Framework.
func (a *Gin) Register(method, path string, h any) (err error) {
	hf, ok := h.(gin.HandlerFunc)
	if !ok {
		return fmt.Errorf("apidoc: expected gin.HandlerFunc, got %T", h)
	}
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("apidoc: gin register %s %s: %v", method, path, p)
		}
	}()
	a.e.Handle(method, path, hf)
	return nil
}

// Mount implements Framework.
func (a *Gin) Mount(prefix string, h http.Handler) {
	a.e.Any(prefix, gin.WrapH(http.StripPrefix(prefix, h)))
	a.e.Any(prefix+"/*path", gin.WrapH(http.StripPrefix(prefix, h)))
}

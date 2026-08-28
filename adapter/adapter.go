// Package adapter adapts the doc server to host web frameworks.
// No framework imports live in this file.
package adapter

import (
	"fmt"
	"net/http"
)

// handler type-asserts h to the framework's handler type, naming the
// expected type on mismatch.
func handler[T any](h any) (T, error) {
	var zero T
	if v, ok := h.(T); ok {
		return v, nil
	}
	return zero, fmt.Errorf("apidoc: expected %T, got %T", zero, h)
}

// recoverRegister turns a framework registration panic into a returned
// error carrying the route.
func recoverRegister(name, method, path string, err *error) {
	if p := recover(); p != nil {
		*err = fmt.Errorf("apidoc: %s register %s %s: %v", name, method, path, p)
	}
}

// Framework is the minimal surface a host framework must provide.
// The concrete handlers (http.HandlerFunc, gin.HandlerFunc, ...) are
// registered with the framework's own types; Register type-asserts h.
type Framework interface {
	// Register attaches h to the framework at method+path.
	// h must be this framework's handler type; the returned error names
	// the expected type on mismatch.
	Register(method, path string, h any) error
	// Mount attaches the doc UI+API handler under prefix. Both {prefix}
	// and {prefix}/anything must be served.
	Mount(prefix string, h http.Handler)
}

// Package apidoc declares the user-facing documentation types.
// The types are aliases of the internal model so a registered Doc can be
// stored and serialized without conversion.
package apidoc

import "github.com/erikwang2013/apidoc-go/model"

// Route pairs an endpoint's documentation with the handler registered on
// the host framework. Handler is the framework's own handler type
// (gin.HandlerFunc, echo.HandlerFunc, ...); the adapter type-asserts it.
type Route struct {
	Method  string // GET/POST/PUT/PATCH/DELETE
	URL     string
	Handler any
	Doc     Doc
}

// Doc declares documentation for one endpoint.
type Doc = model.Doc

// Param describes one input of an endpoint.
type Param = model.Param

// Response describes one possible response of an endpoint.
type Response = model.Response

package apidoc

import (
	"context"

	"github.com/erikwang2013/apidoc-go/debug"
)

type (
	Debugger       = debug.Debugger
	DebugSpec      = debug.Spec
	DebugField     = debug.Field
	DebugValue     = debug.Value
	DebugRequest   = debug.Request
	DebugResponse  = debug.Response
	DebugEvent     = debug.Event
	StreamDebugger = debug.StreamDebugger
	Session        = debug.Session
)

// DebuggerFunc adapts a function to a Debugger.
func DebuggerFunc(protocol string, spec DebugSpec, fn func(context.Context, DebugRequest) (*DebugResponse, error)) Debugger {
	return debug.Func(protocol, spec, fn)
}

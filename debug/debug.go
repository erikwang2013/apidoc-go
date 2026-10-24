// Package debug defines the protocol-agnostic debug extension point.
// A protocol implementation supplies Spec (panel shape) and Exec
// (one request/response round trip); the doc server owns auth, limits
// and rendering.
package debug

import (
	"context"
	"time"
)

const (
	ModeBrowser = "browser"
	ModeServer  = "server"

	ViewAuto       = "auto"
	ViewHTTP       = "http"
	ViewJSON       = "json"
	ViewXML        = "xml"
	ViewText       = "text"
	ViewHex        = "hex"
	ViewTranscript = "transcript"
)

// Value is one documented parameter with the value typed in the UI.
type Value struct {
	Name  string `json:"name"`
	In    string `json:"in"`
	Value string `json:"value"`
}

// Request is one debug execution.
type Request struct {
	ActionID string            `json:"action_id,omitempty"`
	Method   string            `json:"method"`
	URL      string            `json:"url"`
	Params   []Value           `json:"params,omitempty"`
	Body     []byte            `json:"body,omitempty"`
	Encoding string            `json:"encoding,omitempty"`
	Headers  map[string]string `json:"headers,omitempty"`
	Options  map[string]string `json:"options,omitempty"`
}

// Event is one transcript entry for streaming or multi-response protocols.
type Event struct {
	Time      time.Time `json:"time"`
	Direction string    `json:"direction"` // "in" | "out"
	Name      string    `json:"name,omitempty"`
	Data      string    `json:"data,omitempty"`
	Encoding  string    `json:"encoding,omitempty"`
}

// Response is the adapter result.
type Response struct {
	Status   string            `json:"status"`
	Headers  map[string]string `json:"headers,omitempty"`
	Body     []byte            `json:"body,omitempty"`
	Encoding string            `json:"encoding,omitempty"`
	Events   []Event           `json:"events,omitempty"`
	Duration time.Duration     `json:"duration,omitempty"`
}

// Field is a debug-only connection input rendered above the message.
type Field struct {
	Name        string   `json:"name"`
	Label       string   `json:"label,omitempty"`
	Type        string   `json:"type,omitempty"` // text|password|number|bool|select|textarea
	Default     string   `json:"default,omitempty"`
	Placeholder string   `json:"placeholder,omitempty"`
	Required    bool     `json:"required,omitempty"`
	Secret      bool     `json:"secret,omitempty"`
	Options     []string `json:"options,omitempty"`
}

// Spec tells the UI how to render the debug panel for one protocol.
type Spec struct {
	Mode                string   `json:"mode"`
	Actions             []string `json:"actions,omitempty"`
	Encodings           []string `json:"encodings,omitempty"`
	ConnectFields       []Field  `json:"connect_fields,omitempty"`
	BodyIn              string   `json:"body_in,omitempty"`       // default "payload"
	ResponseView        string   `json:"response_view,omitempty"` // default "auto"
	AllowTargetOverride bool     `json:"allow_target_override,omitempty"`
}

// Debugger connects one protocol. Implementations usually wrap a
// client the host application already owns. Exec must return when ctx
// is done; the server relies on that for its timeout.
type Debugger interface {
	Protocol() string
	Spec() Spec
	Exec(context.Context, Request) (*Response, error)
}

type funcDebugger struct {
	protocol string
	spec     Spec
	fn       func(context.Context, Request) (*Response, error)
}

func (d funcDebugger) Protocol() string { return d.protocol }
func (d funcDebugger) Spec() Spec       { return d.spec }
func (d funcDebugger) Exec(ctx context.Context, req Request) (*Response, error) {
	return d.fn(ctx, req)
}

// Func adapts a function to a Debugger.
func Func(protocol string, spec Spec, fn func(context.Context, Request) (*Response, error)) Debugger {
	return funcDebugger{protocol: protocol, spec: spec, fn: fn}
}

// HTTPBrowserSpec is the built-in HTTP/HTTPS panel. The browser executes
// the request, so no server-side Exec is registered.
func HTTPBrowserSpec() Spec {
	return Spec{
		Mode:         ModeBrowser,
		Actions:      []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"},
		Encodings:    []string{"json", "xml", "text"},
		ResponseView: ViewHTTP,
	}
}

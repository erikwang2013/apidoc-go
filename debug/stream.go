package debug

import "context"

// StreamDebugger is a Debugger that keeps a session open for streaming
// or subscription-style protocols.
type StreamDebugger interface {
	Debugger
	Open(ctx context.Context, req Request) (Session, error)
}

// Session is one open debug connection. Send and Events must honor ctx;
// Events is closed when the session ends.
type Session interface {
	Send(ctx context.Context, msg []byte, meta map[string]string) error
	Events() <-chan Event
	Close() error
}

// Package debugtest provides helpers for testing protocol debug adapters
// without starting a doc server.
package debugtest

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/erikwang2013/apidoc-go/debug"
)

// FakeAdapter is a scriptable debug.Debugger that records every request.
// ExecFunc takes precedence over Err, which takes precedence over Response;
// with none set it returns {"OK", nil}.
type FakeAdapter struct {
	Proto     string
	SpecValue debug.Spec
	Response  *debug.Response
	Err       error
	ExecFunc  func(context.Context, debug.Request) (*debug.Response, error)

	mu   sync.Mutex
	reqs []debug.Request
}

func (f *FakeAdapter) Protocol() string {
	if f.Proto != "" {
		return f.Proto
	}
	return "fake"
}

func (f *FakeAdapter) Spec() debug.Spec { return f.SpecValue }

func (f *FakeAdapter) Exec(ctx context.Context, req debug.Request) (*debug.Response, error) {
	f.mu.Lock()
	f.reqs = append(f.reqs, req)
	f.mu.Unlock()
	if f.ExecFunc != nil {
		return f.ExecFunc(ctx, req)
	}
	if f.Err != nil {
		return nil, f.Err
	}
	if f.Response != nil {
		return f.Response, nil
	}
	return &debug.Response{Status: "OK"}, nil
}

// Requests returns a copy of the recorded requests.
func (f *FakeAdapter) Requests() []debug.Request {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]debug.Request(nil), f.reqs...)
}

// LastRequest returns the most recently recorded request.
func (f *FakeAdapter) LastRequest() (debug.Request, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.reqs) == 0 {
		return debug.Request{}, false
	}
	return f.reqs[len(f.reqs)-1], true
}

// RunExec runs one Exec with a bounded context and fails the test when the
// adapter errors or returns nil.
func RunExec(t *testing.T, d debug.Debugger, req debug.Request) *debug.Response {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	resp, err := d.Exec(ctx, req)
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if resp == nil {
		t.Fatal("Exec returned nil response")
	}
	return resp
}

// RequireSpec validates the adapter's protocol and panel spec, then returns
// the spec. It fails the test when the spec would break the debug UI.
func RequireSpec(t *testing.T, d debug.Debugger) debug.Spec {
	t.Helper()
	if err := validateSpec(d); err != nil {
		t.Fatalf("debug adapter: %v", err)
	}
	return d.Spec()
}

func validateSpec(d debug.Debugger) error {
	if strings.TrimSpace(d.Protocol()) == "" {
		return errors.New("empty protocol")
	}
	s := d.Spec()
	switch s.Mode {
	case "", debug.ModeServer, debug.ModeBrowser:
	default:
		return fmt.Errorf("unknown mode %q", s.Mode)
	}
	switch s.ResponseView {
	case "", debug.ViewAuto, debug.ViewHTTP, debug.ViewJSON, debug.ViewXML,
		debug.ViewText, debug.ViewHex, debug.ViewTranscript:
	default:
		return fmt.Errorf("unknown response view %q", s.ResponseView)
	}
	if s.Streaming {
		if _, ok := d.(debug.StreamDebugger); !ok {
			return errors.New("streaming spec but adapter is not a StreamDebugger")
		}
	}
	for lang, tmpl := range s.Snippets {
		if strings.TrimSpace(lang) == "" {
			return errors.New("empty snippet language")
		}
		if strings.TrimSpace(tmpl) == "" {
			return fmt.Errorf("empty %q snippet", lang)
		}
	}
	return nil
}

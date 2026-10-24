package debug

import (
	"context"
	"testing"
)

type fakeSession struct{}

func (fakeSession) Send(context.Context, []byte, map[string]string) error { return nil }
func (fakeSession) Events() <-chan Event                                  { return nil }
func (fakeSession) Close() error                                          { return nil }

type fakeStream struct{}

func (fakeStream) Protocol() string                                 { return "fake" }
func (fakeStream) Spec() Spec                                       { return Spec{Streaming: true} }
func (fakeStream) Exec(context.Context, Request) (*Response, error) { return nil, nil }
func (fakeStream) Open(context.Context, Request) (Session, error)   { return fakeSession{}, nil }

func TestStreamDebugger(t *testing.T) {
	var d StreamDebugger = fakeStream{}
	if !d.Spec().Streaming {
		t.Fatal("streaming flag not set")
	}
	s, err := d.Open(context.Background(), Request{})
	if err != nil || s == nil {
		t.Fatalf("open: %v %v", s, err)
	}
	if err := s.Send(context.Background(), []byte("x"), nil); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
}

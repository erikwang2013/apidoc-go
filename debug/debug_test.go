package debug

import (
	"context"
	"testing"
)

func TestFuncDebugger(t *testing.T) {
	d := Func("mqtt", Spec{Mode: ModeServer, Actions: []string{"PUBLISH"}},
		func(ctx context.Context, req Request) (*Response, error) {
			if req.Method != "PUBLISH" || req.URL != "mqtt://broker/topic" {
				t.Errorf("request not passed through: %+v", req)
			}
			return &Response{Status: "PUBACK", Body: []byte("ok"), Encoding: "text"}, nil
		})
	if d.Protocol() != "mqtt" {
		t.Fatalf("protocol = %q", d.Protocol())
	}
	if d.Spec().Mode != ModeServer || d.Spec().Actions[0] != "PUBLISH" {
		t.Fatalf("spec = %+v", d.Spec())
	}
	resp, err := d.Exec(context.Background(), Request{Method: "PUBLISH", URL: "mqtt://broker/topic"})
	if err != nil || resp.Status != "PUBACK" || string(resp.Body) != "ok" {
		t.Fatalf("exec = %+v, %v", resp, err)
	}
}

func TestHTTPBrowserSpec(t *testing.T) {
	s := HTTPBrowserSpec()
	if s.Mode != ModeBrowser {
		t.Fatalf("mode = %q", s.Mode)
	}
	found := false
	for _, a := range s.Actions {
		if a == "GET" {
			found = true
		}
	}
	if !found {
		t.Fatalf("GET missing from %+v", s.Actions)
	}
}

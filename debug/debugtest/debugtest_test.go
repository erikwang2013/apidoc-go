package debugtest

import (
	"context"
	"errors"
	"testing"

	"github.com/erikwang2013/apidoc-go/debug"
)

func TestFakeAdapterRecordsAndResponds(t *testing.T) {
	f := &FakeAdapter{
		Proto:     "mqtt",
		SpecValue: debug.Spec{Mode: debug.ModeServer, Actions: []string{"PUBLISH"}},
		Response:  &debug.Response{Status: "PUBACK", Body: []byte("ok")},
	}
	if f.Protocol() != "mqtt" {
		t.Fatalf("protocol = %q", f.Protocol())
	}
	resp := RunExec(t, f, debug.Request{ActionID: "a", Method: "PUBLISH", URL: "mqtt://b/t"})
	if resp.Status != "PUBACK" || string(resp.Body) != "ok" {
		t.Fatalf("response = %+v", resp)
	}
	got, ok := f.LastRequest()
	if !ok || got.ActionID != "a" || got.URL != "mqtt://b/t" {
		t.Fatalf("request not recorded: %+v", got)
	}
	if len(f.Requests()) != 1 {
		t.Fatalf("requests = %d", len(f.Requests()))
	}
}

func TestFakeAdapterPrecedence(t *testing.T) {
	f := &FakeAdapter{Err: errors.New("boom"), Response: &debug.Response{Status: "ignored"}}
	if _, err := f.Exec(context.Background(), debug.Request{}); err == nil {
		t.Fatal("Err ignored")
	}
	f.ExecFunc = func(context.Context, debug.Request) (*debug.Response, error) {
		return &debug.Response{Status: "FUNC"}, nil
	}
	resp, err := f.Exec(context.Background(), debug.Request{})
	if err != nil || resp.Status != "FUNC" {
		t.Fatalf("ExecFunc ignored: %+v %v", resp, err)
	}
	var empty FakeAdapter
	if resp := RunExec(t, &empty, debug.Request{}); resp.Status != "OK" || empty.Protocol() != "fake" {
		t.Fatalf("defaults wrong: %+v %q", resp, empty.Protocol())
	}
}

func TestRequireSpec(t *testing.T) {
	good := &FakeAdapter{SpecValue: debug.Spec{
		Mode:         debug.ModeServer,
		ResponseView: debug.ViewTranscript,
		Snippets:     map[string]string{"Go": "send({{body}})"},
	}}
	if spec := RequireSpec(t, good); spec.Mode != debug.ModeServer {
		t.Fatalf("spec = %+v", spec)
	}
	if err := validateSpec(good); err != nil {
		t.Fatalf("valid spec rejected: %v", err)
	}
}

func TestValidateSpecErrors(t *testing.T) {
	cases := []struct {
		name string
		d    debug.Debugger
	}{
		{"empty protocol", &FakeAdapter{Proto: " "}},
		{"bad mode", &FakeAdapter{SpecValue: debug.Spec{Mode: "stream"}}},
		{"bad view", &FakeAdapter{SpecValue: debug.Spec{ResponseView: "yaml"}}},
		{"streaming without session", &FakeAdapter{SpecValue: debug.Spec{Streaming: true}}},
		{"empty snippet language", &FakeAdapter{SpecValue: debug.Spec{Snippets: map[string]string{" ": "x"}}}},
		{"empty snippet", &FakeAdapter{SpecValue: debug.Spec{Snippets: map[string]string{"Go": " "}}}},
	}
	for _, tc := range cases {
		if err := validateSpec(tc.d); err == nil {
			t.Errorf("%s: accepted", tc.name)
		}
	}
}

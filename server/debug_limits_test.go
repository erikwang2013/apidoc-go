package server

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/erikwang2013/apidoc-go/debug"
	"github.com/erikwang2013/apidoc-go/store"
)

func TestDebugOptionCap(t *testing.T) {
	d := &captureDebugger{spec: debug.Spec{
		Mode:          debug.ModeServer,
		Actions:       []string{"PUBLISH"},
		ConnectFields: []debug.Field{{Name: "cert", Type: "file"}},
	}}
	h := Handler(Opts{Prefix: "/apidoc", Store: newDebugStore(),
		Debuggers: map[string]debug.Debugger{"mqtt": d}, DebugAllowUnauth: true})
	id := store.ActionID("a", "v1", "PUBLISH", "mqtt://broker/topic")
	req := debugExecReq{ID: id, Method: "PUBLISH", URL: "mqtt://broker/topic"}

	// A file at the documented 256 KiB raw limit arrives base64-encoded
	// (~342 KiB); the option cap must fit it.
	atLimit := base64.StdEncoding.EncodeToString(make([]byte, 256<<10))
	req.Options = map[string]string{"cert": atLimit}
	b, _ := json.Marshal(req)
	if rec := debugPost(t, h, string(b), true); rec.Code != http.StatusOK {
		t.Fatalf("256 KiB raw file status = %d: %s", rec.Code, rec.Body.String())
	}
	if got := d.got.Options["cert"]; got != atLimit {
		t.Fatalf("option length = %d, want %d", len(got), len(atLimit))
	}

	// Invalid base64 is a bad request, not a size-limit error.
	req.Options = map[string]string{"cert": "not-base64!!"}
	b, _ = json.Marshal(req)
	if rec := debugPost(t, h, string(b), true); rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid base64 status = %d", rec.Code)
	}

	// A declared file field is capped at 256 KiB raw even when its base64
	// form still fits the generic option cap.
	raw260 := base64.StdEncoding.EncodeToString(make([]byte, 260<<10))
	req.Options = map[string]string{"cert": raw260}
	b, _ = json.Marshal(req)
	if rec := debugPost(t, h, string(b), true); rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("260 KiB raw file status = %d", rec.Code)
	}

	// A larger file no longer fits the encoded option cap.
	tooBig := base64.StdEncoding.EncodeToString(make([]byte, 300<<10))
	req.Options = map[string]string{"cert": tooBig}
	b, _ = json.Marshal(req)
	if rec := debugPost(t, h, string(b), true); rec.Code != http.StatusBadRequest {
		t.Fatalf("oversized file status = %d", rec.Code)
	}

	// Response headers keep their tighter 8 KiB cap.
	req.Options = nil
	req.Headers = map[string]string{"X": strings.Repeat("v", maxDebugHeaderValue)}
	b, _ = json.Marshal(req)
	if rec := debugPost(t, h, string(b), true); rec.Code != http.StatusBadRequest {
		t.Fatalf("oversized header status = %d", rec.Code)
	}
}

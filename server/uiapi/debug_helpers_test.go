package uiapi

import (
	"net/http"
	"testing"

	"github.com/erikwang2013/apidoc-go/model"
	"github.com/erikwang2013/apidoc-go/store"
)

func debugStoreHandler(t *testing.T, protocol, method, target string, cfg DebugConfig, extraOpts ...map[string]string) (http.Handler, string) {
	t.Helper()
	st := store.New("Debug")
	st.SetAppMeta("default", "Default")
	opts := map[string]string{"target": target}
	for _, extra := range extraOpts {
		for key, value := range extra {
			opts[key] = value
		}
	}
	st.Register(method, target, model.Doc{
		App: "default", Version: "v1", Controller: "debug", Title: "Debug",
		Protocol:     protocol,
		ProtocolOpts: opts,
		Params:       []model.Param{{Name: "message", In: "body", Type: "string", Field: "1"}},
	})
	return newHandlerWith(t, Options{Store: st, Debug: cfg}), store.ActionID("default", "v1", method, target)
}

func debugSendData(t *testing.T, h http.Handler, actionID, input string) map[string]any {
	t.Helper()
	rec := do(t, h, "POST", "/ui/v1/debugSend",
		`{"appKey":"default|v1","path":"`+actionID+`","input":`+input+`}`, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("debugSend: %d %s", rec.Code, rec.Body.String())
	}
	return decodeData(t, rec)
}

func debugStartData(t *testing.T, h http.Handler, actionID, input string) map[string]any {
	t.Helper()
	rec := do(t, h, "POST", "/ui/v1/debugStart",
		`{"appKey":"default|v1","path":"`+actionID+`","input":`+input+`}`, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("debugStart: %d %s", rec.Code, rec.Body.String())
	}
	return decodeData(t, rec)
}

func debugRPCHandler(t *testing.T, protocol, method, target string, params, returned []model.Param, cfg DebugConfig, extra map[string]string) (http.Handler, string) {
	t.Helper()
	st := store.New("Debug")
	st.SetAppMeta("default", "Default")
	opts := map[string]string{"target": target}
	for key, value := range extra {
		opts[key] = value
	}
	doc := model.Doc{
		App: "default", Version: "v1", Controller: "debug", Title: "Debug",
		Protocol: protocol, ProtocolOpts: opts, Params: params,
	}
	if len(returned) > 0 {
		doc.Responses = []model.Response{{Name: "ok", Fields: returned}}
	}
	st.Register(method, target, doc)
	return newHandlerWith(t, Options{Store: st, Debug: cfg}), store.ActionID("default", "v1", method, target)
}

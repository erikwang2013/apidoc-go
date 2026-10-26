//go:build apidoc_debug_industrial || apidoc_debug_all

package uiapi

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/erikwang2013/apidoc-go/auth"
	"github.com/erikwang2013/apidoc-go/model"
	"github.com/erikwang2013/apidoc-go/store"
)

// DNP3 and IEC104 are unary-only, so scoping is exercised through debugSend
// (debugStart would fail with "stream executor not compiled").
func TestDebugDNP3ShareAndAppScope(t *testing.T) {
	st := newMultiAppStore()
	target := "dnp3://127.0.0.1:1"
	st.Register("GET", target, model.Doc{
		App: "default", Version: "v1", Controller: "debug", Title: "DNP3",
		Protocol: "dnp3", ProtocolOpts: map[string]string{"target": target},
	})
	h := newHandlerWith(t, Options{
		Store:  st,
		Auth:   auth.Config{Enable: true, Password: "pw", Secret: "secret", Expire: time.Hour},
		Debug:  DebugConfig{EnableServerSide: true, AllowPrivateNetworks: true, TimeoutMS: 200},
		AppPWs: map[string]string{"b": "bpw"},
	})
	dnp3ID := store.ActionID("default", "v1", "GET", target)
	itemsID := store.ActionID("default", "v1", "GET", "/items")

	// App scope: an app-b token cannot execute the default app's DNP3 endpoint.
	bToken := loginApp(t, h, "b|v1", "bpw")
	body := `{"appKey":"default|v1","path":"` + dnp3ID + `","input":{}}`
	if rec := do(t, h, "POST", "/ui/v1/debugSend", body, bToken); rec.Code != http.StatusUnauthorized {
		t.Fatalf("cross-app DNP3 token: %d %s", rec.Code, rec.Body.String())
	}

	admin := login(t, h)
	if !decodeDataOf[bool](t, do(t, h, "POST", "/ui/v1/addApiShare",
		`{"key":"dnp3-out","name":"Other","type":"api","appKeys":["default"],"apiKeys":["`+itemsID+`"],"password":"sp"}`, admin)) {
		t.Fatal("add out-of-scope share failed")
	}
	outsideToken := decodeData(t, do(t, h, "POST", "/ui/v1/verifyAuth",
		`{"shareKey":"dnp3-out","password":"sp"}`, ""))["token"].(string)
	outside := `{"appKey":"default|v1","path":"` + dnp3ID + `","shareKey":"dnp3-out","input":{}}`
	if rec := do(t, h, "POST", "/ui/v1/debugSend", outside, outsideToken); rec.Code != http.StatusNotFound {
		t.Fatalf("DNP3 share outside scope: %d %s", rec.Code, rec.Body.String())
	}

	if !decodeDataOf[bool](t, do(t, h, "POST", "/ui/v1/addApiShare",
		`{"key":"dnp3-in","name":"DNP3","type":"api","appKeys":["default"],"apiKeys":["`+dnp3ID+`"],"password":"sp"}`, admin)) {
		t.Fatal("add in-scope share failed")
	}
	insideToken := decodeData(t, do(t, h, "POST", "/ui/v1/verifyAuth",
		`{"shareKey":"dnp3-in","password":"sp"}`, ""))["token"].(string)
	inside := `{"appKey":"default|v1","path":"` + dnp3ID + `","shareKey":"dnp3-in","input":{}}`
	data := decodeData(t, do(t, h, "POST", "/ui/v1/debugSend", inside, insideToken))
	if data["ok"] != false || !strings.Contains(data["error"].(string), "connect failed") {
		t.Fatalf("DNP3 share inside scope should reach the executor: %+v", data)
	}
}

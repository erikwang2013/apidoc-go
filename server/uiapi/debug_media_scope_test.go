//go:build apidoc_debug_media || apidoc_debug_all

package uiapi

import (
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/erikwang2013/apidoc-go/auth"
	"github.com/erikwang2013/apidoc-go/model"
	"github.com/erikwang2013/apidoc-go/store"
)

// RTP is unary-only, so scoping is exercised through debugSend. The target is
// an ephemeral loopback UDP port allowed by the test config.
func TestDebugRTPShareAndAppScope(t *testing.T) {
	socket, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { socket.Close() })
	target := "rtp://" + socket.LocalAddr().String()

	st := newMultiAppStore()
	st.Register("GET", target, model.Doc{
		App: "default", Version: "v1", Controller: "debug", Title: "RTP",
		Protocol: "rtp", ProtocolOpts: map[string]string{"target": target},
	})
	h := newHandlerWith(t, Options{
		Store:  st,
		Auth:   auth.Config{Enable: true, Password: "pw", Secret: "secret", Expire: time.Hour},
		Debug:  DebugConfig{EnableServerSide: true, AllowPrivateNetworks: true, TimeoutMS: 500},
		AppPWs: map[string]string{"b": "bpw"},
	})
	rtpID := store.ActionID("default", "v1", "GET", target)
	itemsID := store.ActionID("default", "v1", "GET", "/items")

	// App scope: an app-b token cannot execute the default app's RTP endpoint.
	bToken := loginApp(t, h, "b|v1", "bpw")
	body := `{"appKey":"default|v1","path":"` + rtpID + `","input":{}}`
	if rec := do(t, h, "POST", "/ui/v1/debugSend", body, bToken); rec.Code != http.StatusUnauthorized {
		t.Fatalf("cross-app RTP token: %d %s", rec.Code, rec.Body.String())
	}

	admin := login(t, h)
	if !decodeDataOf[bool](t, do(t, h, "POST", "/ui/v1/addApiShare",
		`{"key":"rtp-out","name":"Other","type":"api","appKeys":["default"],"apiKeys":["`+itemsID+`"],"password":"sp"}`, admin)) {
		t.Fatal("add out-of-scope share failed")
	}
	outsideToken := decodeData(t, do(t, h, "POST", "/ui/v1/verifyAuth",
		`{"shareKey":"rtp-out","password":"sp"}`, ""))["token"].(string)
	outside := `{"appKey":"default|v1","path":"` + rtpID + `","shareKey":"rtp-out","input":{}}`
	if rec := do(t, h, "POST", "/ui/v1/debugSend", outside, outsideToken); rec.Code != http.StatusNotFound {
		t.Fatalf("RTP share outside scope: %d %s", rec.Code, rec.Body.String())
	}

	if !decodeDataOf[bool](t, do(t, h, "POST", "/ui/v1/addApiShare",
		`{"key":"rtp-in","name":"RTP","type":"api","appKeys":["default"],"apiKeys":["`+rtpID+`"],"password":"sp"}`, admin)) {
		t.Fatal("add in-scope share failed")
	}
	insideToken := decodeData(t, do(t, h, "POST", "/ui/v1/verifyAuth",
		`{"shareKey":"rtp-in","password":"sp"}`, ""))["token"].(string)
	inside := `{"appKey":"default|v1","path":"` + rtpID + `","shareKey":"rtp-in","input":{"fields":{"payload_type":"96"}}}`
	data := decodeData(t, do(t, h, "POST", "/ui/v1/debugSend", inside, insideToken))
	if data["ok"] != true {
		t.Fatalf("RTP share inside scope should reach the executor: %+v", data)
	}
	headers := data["headers"].(map[string]any)
	if headers["x-sent"] != "1" {
		t.Fatalf("RTP send headers: %+v", headers)
	}
}

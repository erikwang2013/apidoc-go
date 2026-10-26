//go:build apidoc_debug_sockets || apidoc_debug_all

package uiapi

import (
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/erikwang2013/apidoc-go/auth"
	"github.com/erikwang2013/apidoc-go/model"
	"github.com/erikwang2013/apidoc-go/store"
)

func TestDebugSessionScopeOnPollAndStop(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(conn net.Conn) {
				defer conn.Close()
				_, _ = io.Copy(io.Discard, conn)
			}(conn)
		}
	}()

	target := "tcp://" + ln.Addr().String()
	st := newMultiAppStore()
	st.Register("POST", target, model.Doc{
		App: "default", Version: "v1", Controller: "debug", Title: "TCP",
		Protocol:     "tcp",
		ProtocolOpts: map[string]string{"target": target},
	})
	tcpID := store.ActionID("default", "v1", "POST", target)
	h := newHandlerWith(t, Options{
		Store:  st,
		Auth:   auth.Config{Enable: true, Password: "pw", Secret: "secret", Expire: time.Hour},
		AppPWs: map[string]string{"b": "bpw"},
		Debug:  DebugConfig{EnableServerSide: true, AllowPrivateNetworks: true, TimeoutMS: 2000},
	})
	admin := login(t, h)
	bToken := loginApp(t, h, "b|v1", "bpw")
	if !decodeDataOf[bool](t, do(t, h, "POST", "/ui/v1/addApiShare",
		`{"key":"s3","name":"Debug","type":"api","appKeys":["default"],"apiKeys":["`+tcpID+`"],"password":"sp"}`, admin)) {
		t.Fatal("add share failed")
	}
	shareToken := decodeData(t, do(t, h, "POST", "/ui/v1/verifyAuth",
		`{"shareKey":"s3","password":"sp"}`, ""))["token"].(string)

	// Share session: wrong shareKey or an app token cannot poll/stop it.
	start := decodeData(t, do(t, h, "POST", "/ui/v1/debugStart",
		`{"appKey":"default|v1","path":"`+tcpID+`","shareKey":"s3","input":{"payload":"x"}}`, shareToken))
	sessionID, _ := start["sessionId"].(string)
	if start["ok"] != true || sessionID == "" {
		t.Fatalf("share start: %+v", start)
	}
	pollBody := func(shareKey string) string {
		return `{"sessionId":"` + sessionID + `","appKey":"default|v1","shareKey":"` + shareKey + `"}`
	}
	if rec := do(t, h, "POST", "/ui/v1/debugPoll", pollBody("other"), shareToken); rec.Code != http.StatusUnauthorized {
		t.Fatalf("wrong share poll: %d %s", rec.Code, rec.Body.String())
	}
	if rec := do(t, h, "POST", "/ui/v1/debugPoll", pollBody(""), bToken); rec.Code != http.StatusUnauthorized {
		t.Fatalf("app token share poll: %d %s", rec.Code, rec.Body.String())
	}
	if data := decodeData(t, do(t, h, "POST", "/ui/v1/debugPoll", pollBody("s3"), shareToken)); data["ok"] != true {
		t.Fatalf("share poll: %+v", data)
	}
	if rec := do(t, h, "POST", "/ui/v1/debugStop", pollBody("other"), shareToken); rec.Code != http.StatusUnauthorized {
		t.Fatalf("wrong share stop: %d %s", rec.Code, rec.Body.String())
	}
	if data := decodeData(t, do(t, h, "POST", "/ui/v1/debugStop", pollBody("s3"), shareToken)); data["ok"] != true {
		t.Fatalf("share stop: %+v", data)
	}

	// Token session: another app's token cannot poll/stop it.
	start = decodeData(t, do(t, h, "POST", "/ui/v1/debugStart",
		`{"appKey":"default|v1","path":"`+tcpID+`","input":{"payload":"x"}}`, admin))
	sessionID, _ = start["sessionId"].(string)
	if start["ok"] != true || sessionID == "" {
		t.Fatalf("token start: %+v", start)
	}
	appBody := `{"sessionId":"` + sessionID + `","appKey":"default|v1"}`
	if rec := do(t, h, "POST", "/ui/v1/debugPoll", appBody, bToken); rec.Code != http.StatusUnauthorized {
		t.Fatalf("cross-app poll: %d %s", rec.Code, rec.Body.String())
	}
	if rec := do(t, h, "POST", "/ui/v1/debugStop", appBody, bToken); rec.Code != http.StatusUnauthorized {
		t.Fatalf("cross-app stop: %d %s", rec.Code, rec.Body.String())
	}
	if data := decodeData(t, do(t, h, "POST", "/ui/v1/debugPoll", appBody, admin)); data["ok"] != true {
		t.Fatalf("token poll: %+v", data)
	}
	if data := decodeData(t, do(t, h, "POST", "/ui/v1/debugStop", appBody, admin)); data["ok"] != true {
		t.Fatalf("token stop: %+v", data)
	}
}

//go:build apidoc_debug_mqtt || apidoc_debug_messaging || apidoc_debug_all

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

func mqttScopeRead(conn net.Conn) (byte, []byte, error) {
	head := make([]byte, 1)
	if _, err := io.ReadFull(conn, head); err != nil {
		return 0, nil, err
	}
	length, multiplier := 0, 1
	for i := 0; i < 4; i++ {
		digit := make([]byte, 1)
		if _, err := io.ReadFull(conn, digit); err != nil {
			return 0, nil, err
		}
		length += int(digit[0]&0x7f) * multiplier
		if digit[0]&0x80 == 0 {
			break
		}
		multiplier *= 128
	}
	body := make([]byte, length)
	if _, err := io.ReadFull(conn, body); err != nil {
		return 0, nil, err
	}
	return head[0], body, nil
}

func mqttScopeWrite(conn net.Conn, header byte, body []byte) error {
	packet := []byte{header}
	length := len(body)
	for {
		digit := byte(length % 128)
		length /= 128
		if length > 0 {
			digit |= 0x80
		}
		packet = append(packet, digit)
		if length == 0 {
			break
		}
	}
	_, err := conn.Write(append(packet, body...))
	return err
}

func mqttScopeBroker(t *testing.T) net.Listener {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(conn net.Conn) {
				defer conn.Close()
				if _, _, err := mqttScopeRead(conn); err != nil { // CONNECT
					return
				}
				if mqttScopeWrite(conn, 0x20, []byte{0, 0}) != nil {
					return
				}
				header, body, err := mqttScopeRead(conn) // SUBSCRIBE
				if err != nil || header>>4 != 8 || len(body) < 2 {
					return
				}
				if mqttScopeWrite(conn, 0x90, append(append([]byte{}, body[:2]...), 0)) != nil {
					return
				}
				topic := append([]byte{0, 7}, "demo/t"...)
				if mqttScopeWrite(conn, 0x30, append(topic, "hello"...)) != nil {
					return
				}
				for {
					if _, _, err := mqttScopeRead(conn); err != nil {
						return
					}
				}
			}(conn)
		}
	}()
	return ln
}

func TestDebugMessagingSessionScopeOnPollAndStop(t *testing.T) {
	ln := mqttScopeBroker(t)
	target := "mqtt://" + ln.Addr().String()
	st := newMultiAppStore()
	st.Register("POST", target, model.Doc{
		App: "default", Version: "v1", Controller: "debug", Title: "MQTT",
		Protocol:     "mqtt",
		ProtocolOpts: map[string]string{"target": target, "topic": "demo/t"},
	})
	mqttID := store.ActionID("default", "v1", "POST", target)
	h := newHandlerWith(t, Options{
		Store:  st,
		Auth:   auth.Config{Enable: true, Password: "pw", Secret: "secret", Expire: time.Hour},
		AppPWs: map[string]string{"b": "bpw"},
		Debug:  DebugConfig{EnableServerSide: true, AllowPrivateNetworks: true, TimeoutMS: 2000},
	})
	admin := login(t, h)
	bToken := loginApp(t, h, "b|v1", "bpw")
	if !decodeDataOf[bool](t, do(t, h, "POST", "/ui/v1/addApiShare",
		`{"key":"s3","name":"Debug","type":"api","appKeys":["default"],"apiKeys":["`+mqttID+`"],"password":"sp"}`, admin)) {
		t.Fatal("add share failed")
	}
	shareToken := decodeData(t, do(t, h, "POST", "/ui/v1/verifyAuth",
		`{"shareKey":"s3","password":"sp"}`, ""))["token"].(string)

	// Share session: wrong shareKey or an app token cannot poll/stop it.
	start := decodeData(t, do(t, h, "POST", "/ui/v1/debugStart",
		`{"appKey":"default|v1","path":"`+mqttID+`","shareKey":"s3"}`, shareToken))
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
		`{"appKey":"default|v1","path":"`+mqttID+`"}`, admin))
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

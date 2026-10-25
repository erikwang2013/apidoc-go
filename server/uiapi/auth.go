package uiapi

import (
	"net/http"
	"strings"
	"time"

	"github.com/erikwang2013/apidoc-go/auth"
)

func (h *handler) verifyAuth(w http.ResponseWriter, r *http.Request) {
	body, err := decode[struct {
		Password string `json:"password"`
		AppKey   string `json:"appKey"`
		ShareKey string `json:"shareKey"`
	}](r)
	if err != nil || body.Password == "" {
		writeError(w, http.StatusBadRequest, 400, "invalid body")
		return
	}
	app, _ := splitAppKey(body.AppKey)
	if app != "" {
		if password, found := h.o.AppPWs[app]; found && password != "" {
			if !auth.CheckPassword(h.o.Auth.Secret, password, body.Password) {
				writeError(w, http.StatusUnauthorized, 401, "password error")
				return
			}
			token, err := auth.Issue(h.o.Auth.Secret, appTokenTTL, app)
			if err != nil {
				writeError(w, http.StatusInternalServerError, 500, "token issue failed")
				return
			}
			writeJSON(w, http.StatusOK, ok(map[string]any{
				"token": token, "expire": int(appTokenTTL.Seconds()),
			}))
			return
		}
	}
	if !h.o.Auth.Enable || h.o.Auth.Password == "" {
		writeError(w, http.StatusUnauthorized, 401, "password error")
		return
	}
	if !auth.CheckPassword(h.o.Auth.Secret, h.o.Auth.Password, body.Password) {
		writeError(w, http.StatusUnauthorized, 401, "password error")
		return
	}
	ttl := h.o.Auth.Expire
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	token, err := auth.Issue(h.o.Auth.Secret, ttl, "session")
	if err != nil {
		writeError(w, http.StatusInternalServerError, 500, "token issue failed")
		return
	}
	writeJSON(w, http.StatusOK, ok(map[string]any{
		"token": token, "expire": int(ttl.Seconds()),
	}))
}

// canAccess authorizes UIC-1 reads. A protected app requires its scoped app
// token; a global session can read unprotected apps.
func (h *handler) canAccess(r *http.Request, appKey string) bool {
	if !h.o.Auth.Enable && len(h.o.AppPWs) == 0 {
		return true
	}
	token := bearerToken(r)
	if token == "" {
		return false
	}
	scope, valid := auth.Verify(h.o.Auth.Secret, token)
	if !valid {
		return false
	}
	app, _ := splitAppKey(appKey)
	if app != "" && h.o.AppPWs[app] != "" {
		return scope == app
	}
	if scope == "session" {
		return true
	}
	if app != "" && scope == app {
		return true
	}
	return appKey == "" && scope != ""
}

func bearerToken(r *http.Request) string {
	value := strings.TrimSpace(r.Header.Get("Authorization"))
	if len(value) >= 7 && strings.EqualFold(value[:7], "Bearer ") {
		return strings.TrimSpace(value[7:])
	}
	return strings.TrimSpace(r.Header.Get("X-Apidoc-App-Token"))
}

package uiapi

import (
	"encoding/json"
	"io"
	"net/http"
)

// Phase 1 advertises no relay protocols for Go. If an older UI calls these
// routes anyway, fail closed with an explicit reason instead of a fake success.
// Share-scoped calls are still authorized first so a share token cannot probe
// outside its scope.
func (h *handler) debugUnavailable(w http.ResponseWriter, r *http.Request) {
	var body struct {
		AppKey   string `json:"appKey"`
		ShareKey string `json:"shareKey"`
		Path     string `json:"path"`
	}
	_ = json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&body)
	if body.ShareKey != "" {
		rec, ok := h.shareAuth(w, r, body.ShareKey)
		if !ok {
			return
		}
		if body.AppKey != "" && !h.shareAllowsApp(rec, body.AppKey) {
			writeError(w, http.StatusNotFound, 404, "share not exists")
			return
		}
		if body.Path != "" && !rec.apiAllowed(body.Path) {
			writeError(w, http.StatusForbidden, 403, "share api not allowed")
			return
		}
	}
	writeJSON(w, http.StatusOK, ok(map[string]any{
		"ok":    false,
		"error": "server-side debug is not available in this backend version",
	}))
}

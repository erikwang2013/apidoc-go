package uiapi

import "net/http"

// Phase 1 advertises no relay protocols for Go. If an older UI calls these
// routes anyway, fail closed with an explicit reason instead of a fake success.
func (h *handler) debugUnavailable(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, ok(map[string]any{
		"ok":    false,
		"error": "server-side debug is not available in this backend version",
	}))
}

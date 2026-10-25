// Package uiapi implements the UIC-1 HTTP API for apidoc-go.
package uiapi

import (
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/erikwang2013/apidoc-go/auth"
	"github.com/erikwang2013/apidoc-go/model"
	"github.com/erikwang2013/apidoc-go/store"
)

const contractVersion = "1.0"

// Options wires UIC-1 to the existing doc store and auth config.
type Options struct {
	Store        *store.Store
	Auth         auth.Config
	AppPWs       map[string]string
	GlobalParams []model.Param
	PublicHost   string
	AppHosts     map[string]string
}

type handler struct {
	o Options
}

// Register mounts the UIC-1 routes on mux. Legacy /api routes stay untouched.
func Register(mux *http.ServeMux, o Options) {
	h := &handler{o: o}
	mux.HandleFunc("GET /ui/v1/capabilities", h.capabilities)
	mux.HandleFunc("GET /ui/v1/config", h.config)
	mux.HandleFunc("POST /ui/v1/menus", h.menus)
	mux.HandleFunc("POST /ui/v1/detail", h.detail)
	mux.HandleFunc("GET /ui/v1/docMenus", h.docMenus)
	mux.HandleFunc("POST /ui/v1/docMenus", h.docMenus)
	mux.HandleFunc("POST /ui/v1/docDetail", h.docDetail)
	mux.HandleFunc("POST /ui/v1/verifyAuth", h.verifyAuth)
	mux.HandleFunc("POST /ui/v1/debugSend", h.debugUnavailable)
	mux.HandleFunc("POST /ui/v1/debugStart", h.debugUnavailable)
	mux.HandleFunc("POST /ui/v1/debugPoll", h.debugUnavailable)
	mux.HandleFunc("POST /ui/v1/debugStop", h.debugUnavailable)
}

func ok(data any) map[string]any {
	return map[string]any{"code": 0, "message": "ok", "data": data}
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status, code int, message string) {
	writeJSON(w, status, map[string]any{"code": code, "message": message})
}

func decode[T any](r *http.Request) (T, error) {
	var value T
	err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&value)
	return value, err
}

func (h *handler) capabilitiesData() map[string]any {
	return map[string]any{
		"contract": contractVersion,
		"backend":  "go",
		"version":  "dev",
		"features": map[string]any{
			"menus":               true,
			"detail":              true,
			"docs":                true,
			"protocolDefinitions": true,
			"debug": map[string]any{
				"browser": []string{"http", "websocket", "sse"},
				"relay":   []string{},
				"modes":   []string{"browser-http", "browser-websocket", "browser-sse"},
			},
			"share": map[string]any{"link": true, "records": false, "actions": false},
			"cache": map[string]any{"manage": false, "mode": "server"},
			"generator": map[string]any{
				"forms": false, "templates": false, "sideEffects": false,
			},
			"exports": []string{},
			"auth": map[string]any{
				"global":     h.o.Auth.Enable,
				"app":        len(h.o.AppPWs) > 0,
				"shareToken": false,
			},
		},
	}
}

func (h *handler) capabilities(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, ok(h.capabilitiesData()))
}

const appTokenTTL = time.Hour

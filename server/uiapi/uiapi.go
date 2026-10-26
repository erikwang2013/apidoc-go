// Package uiapi implements the UIC-1 HTTP API for apidoc-go.
package uiapi

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
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
	// ShareDir is where share records live; empty means a directory under
	// os.TempDir().
	ShareDir      string
	Generator     []GeneratorSpec
	CodeTemplates []CodeTemplateSpec
	ShareActions  []ShareActionSpec
	// CacheClear/CacheWarm are wired by server.Handler to the response cache;
	// nil means cache management is unavailable.
	CacheClear func()
	CacheWarm  func()
}

type handler struct {
	o      Options
	shares *shareStore
}

// Register mounts the UIC-1 routes on mux. Legacy /api routes stay untouched.
func Register(mux *http.ServeMux, o Options) {
	if o.ShareDir == "" {
		o.ShareDir = filepath.Join(os.TempDir(), "apidoc-go-shares")
	}
	h := &handler{o: o, shares: &shareStore{dir: o.ShareDir}}
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
	mux.HandleFunc("POST /ui/v1/addApiShare", h.addShare)
	mux.HandleFunc("POST /ui/v1/getApiShareList", h.shareList)
	mux.HandleFunc("POST /ui/v1/getApiShareDetail", h.shareDetail)
	mux.HandleFunc("POST /ui/v1/deleteApiShare", h.deleteShare)
	mux.HandleFunc("POST /ui/v1/handleApiShareAction", h.shareAction)
	mux.HandleFunc("POST /ui/v1/cache/clear", h.cacheClear)
	mux.HandleFunc("POST /ui/v1/cache/warm", h.cacheWarm)
	mux.HandleFunc("POST /ui/v1/generator", h.generator)
	mux.HandleFunc("POST /ui/v1/renderCodeTemplate", h.renderCodeTemplate)
	mux.HandleFunc("POST /ui/v1/exportAllData", h.exportAllData)
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
			"share": map[string]any{"link": true, "records": true, "actions": true},
			"cache": map[string]any{"manage": true, "mode": "server"},
			"generator": map[string]any{
				"forms": true, "templates": true, "sideEffects": false,
			},
			"exports": []string{"md", "pdf", "openapi", "postman", "asyncapi", "typescript"},
			"auth": map[string]any{
				"global":     h.o.Auth.Enable,
				"app":        true, // app-scoped tokens are always supported
				"shareToken": true,
			},
		},
	}
}

func (h *handler) capabilities(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, ok(h.capabilitiesData()))
}

const appTokenTTL = time.Hour

// Package server serves the doc JSON API as a plain http.Handler,
// framework-agnostically.
package server

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/erikwang2013/apidoc-go/auth"
	"github.com/erikwang2013/apidoc-go/export"
	"github.com/erikwang2013/apidoc-go/mock"
	"github.com/erikwang2013/apidoc-go/model"
	"github.com/erikwang2013/apidoc-go/server/uiapi"
	"github.com/erikwang2013/apidoc-go/store"
	"github.com/yuin/goldmark"
)

const (
	cookieName  = "apidoc_token"
	appTokenTTL = time.Hour
)

// Opts wires the server to a store and configuration.
type Opts struct {
	Prefix        string
	Store         *store.Store
	Auth          auth.Config
	AppPWs        map[string]string // app key -> per-app password (overrides global)
	DebugOrigins  []string          // origins allowed CORS access; empty = none
	GlobalParams  []model.Param     // merged into every action's params
	PublicHost    string            // fallback absolute debug base URL
	AppHosts      map[string]string // app key -> absolute debug base URL
	ShareDir      string            // UIC-1 share records directory
	Generator     []uiapi.GeneratorSpec
	CodeTemplates []uiapi.CodeTemplateSpec
	ShareActions  []uiapi.ShareActionSpec
}

// Handler builds the doc JSON API handler: auth middleware, CORS and the
// /api routes. Adapters mount it under their prefix and strip that
// prefix, so all routes here are mount-relative.
func Handler(o Opts) http.Handler {
	if o.Auth.Expire <= 0 {
		o.Auth.Expire = 24 * time.Hour
	}
	mux := http.NewServeMux()
	h := &apiHandler{o: o, cache: map[string]cachedResp{}}
	mux.HandleFunc("GET /api/menus", h.menus)
	mux.HandleFunc("GET /api/protocols", h.protocols)
	mux.HandleFunc("GET /api/detail", h.detail)
	mux.HandleFunc("GET /api/export", h.export)
	mux.HandleFunc("POST /api/login", h.login)
	mux.HandleFunc("POST /api/app-login", h.appLogin)
	uiapi.Register(mux, uiapi.Options{
		Store:         o.Store,
		Auth:          o.Auth,
		AppPWs:        o.AppPWs,
		GlobalParams:  o.GlobalParams,
		PublicHost:    o.PublicHost,
		AppHosts:      o.AppHosts,
		ShareDir:      o.ShareDir,
		Generator:     o.Generator,
		CodeTemplates: o.CodeTemplates,
		ShareActions:  o.ShareActions,
		CacheClear:    h.clearCache,
		CacheWarm:     h.warm,
	})
	h.warm()
	// CORS outermost so even 401s carry the headers the browser needs.
	return withCORS(o, withAuth(o, mux))
}

type apiHandler struct {
	o     Opts
	mu    sync.Mutex
	cache map[string]cachedResp
}

// cachedResp is one marshaled response; version invalidates it when the
// store changes.
type cachedResp struct {
	version uint64
	body    []byte
	etag    string
}

// jsonFor returns the cached JSON for key, rebuilding it when the store
// version changed. build must read the store once and must not use the
// request, so responses can be warmed before the first request.
func (h *apiHandler) jsonFor(key string, build func() (any, error)) (cachedResp, error) {
	v := h.o.Store.Version()
	h.mu.Lock()
	c, ok := h.cache[key]
	h.mu.Unlock()
	if ok && c.version == v {
		return c, nil
	}
	data, err := build()
	if err != nil {
		return cachedResp{}, err
	}
	body, err := json.Marshal(data)
	if err != nil {
		return cachedResp{}, err
	}
	c = cachedResp{version: v, body: body, etag: etagOf(body)}
	h.mu.Lock()
	if h.o.Store.Version() == v {
		h.cache[key] = c
	}
	h.mu.Unlock()
	return c, nil
}

// clearCache drops every cached response; the next request rebuilds it.
func (h *apiHandler) clearCache() {
	h.mu.Lock()
	h.cache = map[string]cachedResp{}
	h.mu.Unlock()
}

// warm precomputes the static payloads so the first request is also fast.
// Registrations made after Handler creation bump the store version and
// rebuild the affected payload on the next request (once).
func (h *apiHandler) warm() {
	_, _ = h.jsonFor("protocols", func() (any, error) {
		return map[string]any{"code": 0, "msg": "ok", "data": model.Protocols()}, nil
	})
	_, _ = h.jsonFor("menus", func() (any, error) { return h.menusPayload(), nil })
	_, _ = h.jsonFor("export:json", h.exportPayload)
	_, _ = h.jsonFor("export:typescript", h.exportTSPayload)
}

// login issues the global session cookie.
func (h *apiHandler) login(w http.ResponseWriter, r *http.Request) {
	var body struct{ Password string }
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		errJSON(w, http.StatusBadRequest, "invalid body")
		return
	}
	if !auth.CheckPassword(h.o.Auth.Secret, h.o.Auth.Password, body.Password) {
		errJSON(w, http.StatusUnauthorized, "invalid password")
		return
	}
	tok, err := auth.Issue(h.o.Auth.Secret, h.o.Auth.Expire, "session")
	if err != nil {
		errJSON(w, http.StatusInternalServerError, "token issue failed")
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: cookieName, Value: tok, Path: h.o.Prefix,
		HttpOnly: true, SameSite: http.SameSiteLaxMode,
		Expires: time.Now().Add(h.o.Auth.Expire), Secure: h.o.Auth.Secure,
	})
	writeJSON(w, r, http.StatusOK, map[string]any{"code": 0, "msg": "ok"})
}

// appLogin issues a short-lived token scoped to one app.
func (h *apiHandler) appLogin(w http.ResponseWriter, r *http.Request) {
	var body struct {
		App      string
		Password string
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		errJSON(w, http.StatusBadRequest, "invalid body")
		return
	}
	pw, ok := h.o.AppPWs[body.App]
	if !ok {
		errJSON(w, http.StatusNotFound, "no such app")
		return
	}
	if !auth.CheckPassword(h.o.Auth.Secret, pw, body.Password) {
		errJSON(w, http.StatusUnauthorized, "invalid password")
		return
	}
	tok, err := auth.Issue(h.o.Auth.Secret, appTokenTTL, body.App)
	if err != nil {
		errJSON(w, http.StatusInternalServerError, "token issue failed")
		return
	}
	writeJSON(w, r, http.StatusOK, map[string]any{
		"code": 0, "msg": "ok",
		"data": map[string]any{"token": tok, "expire": appTokenTTL.Seconds()},
	})
}

// menus returns the app tree with id/title/sort per action.
func (h *apiHandler) menus(w http.ResponseWriter, r *http.Request) {
	c, err := h.jsonFor("menus", func() (any, error) { return h.menusPayload(), nil })
	if err != nil {
		errJSON(w, http.StatusInternalServerError, "encode failed")
		return
	}
	writeCached(w, r, c)
}

func (h *apiHandler) menusPayload() any {
	apps := h.o.Store.Menus()
	out := make([]mApp, 0, len(apps))
	for _, app := range apps {
		ma := mApp{Key: app.Key, Title: app.Title}
		for _, v := range app.Versions {
			mv := mVersion{Name: v.Name}
			for _, c := range v.Controllers {
				mc := mController{Name: c.Name}
				for _, a := range c.Actions {
					mc.Actions = append(mc.Actions, mAction{ID: a.ID, Title: a.Title, Sort: a.Sort})
				}
				sort.SliceStable(mc.Actions, func(i, j int) bool { return mc.Actions[i].Sort < mc.Actions[j].Sort })
				mv.Controllers = append(mv.Controllers, mc)
			}
			ma.Versions = append(ma.Versions, mv)
		}
		out = append(out, ma)
	}
	return map[string]any{"code": 0, "msg": "ok", "data": out}
}

// protocols returns the annotation protocol registry for the external UI.
func (h *apiHandler) protocols(w http.ResponseWriter, r *http.Request) {
	c, err := h.jsonFor("protocols", func() (any, error) {
		return map[string]any{"code": 0, "msg": "ok", "data": model.Protocols()}, nil
	})
	if err != nil {
		errJSON(w, http.StatusInternalServerError, "encode failed")
		return
	}
	writeCached(w, r, c)
}

// detail returns one full action. Actions of password-protected apps
// require a valid scoped app token (X-Apidoc-App-Token header).
func (h *apiHandler) detail(w http.ResponseWriter, r *http.Request) {
	a, ok := h.o.Store.Action(r.URL.Query().Get("id"))
	if !ok {
		errJSON(w, http.StatusNotFound, "action not found")
		return
	}
	protocol := a.Protocol
	if protocol == "" {
		protocol = model.ProtocolOf(a.URL)
	}
	var info *model.ProtocolInfo
	if pi, ok := model.ProtocolInfoOf(protocol); ok {
		info = &pi
	}
	_, protected := h.o.AppPWs[a.App]
	if protected {
		tok := r.Header.Get("X-Apidoc-App-Token")
		if data, ok := auth.Verify(h.o.Auth.Secret, tok); !ok || data != a.App {
			writeJSON(w, r, http.StatusUnauthorized, map[string]any{
				"code": http.StatusUnauthorized, "msg": "app token required", "app": a.App,
			})
			return
		}
	}
	var md bytes.Buffer
	_ = goldmark.Convert([]byte(a.Markdown), &md)
	writeJSON(w, r, http.StatusOK, map[string]any{"code": 0, "msg": "ok", "data": map[string]any{
		"id": a.ID, "app": a.App, "version": a.Version, "controller": a.Controller,
		"method": a.Method, "url": a.URL, "protocol": protocol, "protocol_info": info,
		"title": a.Title, "desc": a.Desc,
		"author": a.Author, "params": mergeParams(h.o.GlobalParams, a.Params),
		"responses": a.Responses, "markdown_html": md.String(), "mock": actionMock(a),
		"protected": protected,
	}})
}

// export serves the whole doc tree: format=json (default) returns the
// project tree, format=typescript returns TypeScript interface
// definitions. Unknown formats get a 400. Auth gating is inherited from
// withAuth like every other /api route.
func (h *apiHandler) export(w http.ResponseWriter, r *http.Request) {
	// Protected apps' payloads are in the export tree; require the same
	// session token withAuth uses before dumping it.
	if len(h.o.AppPWs) > 0 && !validSession(h.o, r) {
		errJSON(w, http.StatusUnauthorized, "login required")
		return
	}
	switch r.URL.Query().Get("format") {
	case "", "json":
		c, err := h.jsonFor("export:json", h.exportPayload)
		if err != nil {
			errJSON(w, http.StatusInternalServerError, "export failed")
			return
		}
		writeCached(w, r, c)
	case "typescript":
		c, err := h.jsonFor("export:typescript", h.exportTSPayload)
		if err != nil {
			errJSON(w, http.StatusInternalServerError, "export failed")
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(c.body)
	default:
		errJSON(w, http.StatusBadRequest, "unknown format")
	}
}

func (h *apiHandler) exportPayload() (any, error) {
	p, err := h.o.Store.Project()
	if err != nil {
		return nil, err
	}
	return map[string]any{"code": 0, "msg": "ok", "data": p}, nil
}

func (h *apiHandler) exportTSPayload() (any, error) {
	p, err := h.o.Store.Project()
	if err != nil {
		return nil, err
	}
	return map[string]any{"code": 0, "msg": "ok", "data": export.Typescript(p)}, nil
}

// actionMock returns the action's own mock string, or generated example
// values keyed by param name when none was registered.
func actionMock(a *model.Action) string {
	if a.Mock != "" {
		return a.Mock
	}
	return mock.Action(a)
}

// mergeParams returns global params with per-action params overriding by
// (name, in).
func mergeParams(global, own []model.Param) []model.Param {
	idx := map[string]int{}
	out := make([]model.Param, 0, len(global)+len(own))
	for _, p := range global {
		idx[key(p)] = len(out)
		out = append(out, p)
	}
	for _, p := range own {
		if i, ok := idx[key(p)]; ok {
			out[i] = p
		} else {
			out = append(out, p)
		}
	}
	return out
}

func key(p model.Param) string { return p.Name + "\x00" + p.In }

func writeJSON(w http.ResponseWriter, r *http.Request, status int, v any) {
	body, err := json.Marshal(v)
	if err != nil {
		body = []byte(`{"code":500,"msg":"encode failed"}`)
		status = http.StatusInternalServerError
	}
	if status < 300 && r != nil && (r.Method == http.MethodGet || r.Method == http.MethodHead) {
		writeCached(w, r, cachedResp{body: body, etag: etagOf(body)})
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

// writeCached writes a 200 JSON response with its precomputed ETag.
// private: detail of token-protected apps must not land in a shared cache.
func writeCached(w http.ResponseWriter, r *http.Request, c cachedResp) {
	w.Header().Set("ETag", c.etag)
	w.Header().Set("Cache-Control", "private, max-age=300")
	if r.Header.Get("If-None-Match") == c.etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(c.body)
}

func etagOf(body []byte) string {
	sum := sha256.Sum256(body)
	return `"` + hex.EncodeToString(sum[:]) + `"`
}

func errJSON(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, nil, status, map[string]any{"code": status, "msg": msg})
}

// validSession reports whether the request carries the global session
// cookie that login issues. Only the "session" token qualifies: an
// app-scoped token (weaker secret holder) must not bypass the gate.
func validSession(o Opts, r *http.Request) bool {
	c, err := r.Cookie(cookieName)
	if err != nil {
		return false
	}
	data, ok := auth.Verify(o.Auth.Secret, c.Value)
	return ok && data == "session"
}

// withAuth gates the /api endpoints when global auth is enabled.
// Login and app-login are the way in; every other /api route requires a
// valid session.
func withAuth(o Opts, next http.Handler) http.Handler {
	if !o.Auth.Enable {
		return next
	}
	open := map[string]bool{"/api/login": true, "/api/app-login": true}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api") || open[r.URL.Path] || validSession(o, r) {
			next.ServeHTTP(w, r)
			return
		}
		errJSON(w, http.StatusUnauthorized, "unauthorized")
	})
}

// withCORS allows exactly the configured origins; nothing for anyone else.
func withCORS(o Opts, next http.Handler) http.Handler {
	if len(o.DebugOrigins) == 0 {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if origin := r.Header.Get("Origin"); slices.Contains(o.DebugOrigins, origin) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Add("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			if r.Method == http.MethodOptions {
				w.Header().Set("Access-Control-Allow-Methods", "GET,POST,PUT,PATCH,DELETE,OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Apidoc-App-Token")
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// menu DTOs: id/title/sort per action, no payload duplication.
type mApp struct {
	Key      string     `json:"key"`
	Title    string     `json:"title"`
	Versions []mVersion `json:"versions"`
}

type mVersion struct {
	Name        string        `json:"name"`
	Controllers []mController `json:"controllers"`
}

type mController struct {
	Name    string    `json:"name"`
	Actions []mAction `json:"actions"`
}

type mAction struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Sort  int    `json:"sort"`
}

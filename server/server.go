// Package server serves the doc JSON API and the embedded UI as a
// plain http.Handler, framework-agnostically.
package server

import (
	"bytes"
	"embed"
	"encoding/json"
	"net/http"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/erikwang2013/apidoc-go/auth"
	"github.com/erikwang2013/apidoc-go/export"
	"github.com/erikwang2013/apidoc-go/mock"
	"github.com/erikwang2013/apidoc-go/model"
	"github.com/erikwang2013/apidoc-go/store"
	"github.com/yuin/goldmark"
)

//go:embed static
var uiFS embed.FS

// indexHTML is the embedded UI. (Read through embed.FS rather than a
// plain []byte embed: some hardened toolchains reject string/[]byte
// go:embed directives.)
var indexHTML = func() []byte {
	b, err := uiFS.ReadFile("static/index.html")
	if err != nil {
		panic("apidoc: embedded UI missing: " + err.Error())
	}
	return b
}()

const (
	cookieName  = "apidoc_token"
	appTokenTTL = time.Hour
)

// Opts wires the server to a store and configuration.
type Opts struct {
	Prefix       string
	Store        *store.Store
	Auth         auth.Config
	AppPWs       map[string]string // app key -> per-app password (overrides global)
	DebugOrigins []string          // origins allowed CORS access; empty = none
	GlobalParams []model.Param     // merged into every action's params
}

// Handler builds the doc server handler: auth middleware, CORS, JSON API
// at /api, embedded UI at /. Adapters mount it under their prefix and
// strip that prefix, so all routes here are mount-relative.
func Handler(o Opts) http.Handler {
	mux := http.NewServeMux()
	h := &apiHandler{o: o}
	mux.HandleFunc("GET /api/menus", h.menus)
	mux.HandleFunc("GET /api/detail", h.detail)
	mux.HandleFunc("GET /api/export", h.export)
	mux.HandleFunc("POST /api/login", h.login)
	mux.HandleFunc("POST /api/app-login", h.appLogin)
	mux.HandleFunc("GET /{$}", h.ui)
	// CORS outermost so even 401s carry the headers the browser needs.
	inner := withCORS(o, withAuth(o, mux))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Adapters strip the mount prefix; the exact {prefix} request
		// then arrives with an empty path, which means the UI.
		if r.URL.Path == "" {
			r.URL.Path = "/"
		}
		inner.ServeHTTP(w, r)
	})
}

type apiHandler struct{ o Opts }

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
	expire := h.o.Auth.Expire
	if expire <= 0 {
		expire = 24 * time.Hour
	}
	tok, err := auth.Issue(h.o.Auth.Secret, expire, "session")
	if err != nil {
		errJSON(w, http.StatusInternalServerError, "token issue failed")
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: cookieName, Value: tok, Path: h.o.Prefix,
		HttpOnly: true, SameSite: http.SameSiteLaxMode,
		Expires: time.Now().Add(expire), Secure: h.o.Auth.Secure,
	})
	writeJSON(w, http.StatusOK, map[string]any{"code": 0, "msg": "ok"})
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
	writeJSON(w, http.StatusOK, map[string]any{
		"code": 0, "msg": "ok",
		"data": map[string]any{"token": tok, "expire": appTokenTTL.Seconds()},
	})
}

// menus returns the app tree with id/title/sort per action.
func (h *apiHandler) menus(w http.ResponseWriter, r *http.Request) {
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
	writeJSON(w, http.StatusOK, map[string]any{"code": 0, "msg": "ok", "data": out})
}

// detail returns one full action. Actions of password-protected apps
// require a valid scoped app token (X-Apidoc-App-Token header).
func (h *apiHandler) detail(w http.ResponseWriter, r *http.Request) {
	a, ok := h.o.Store.Action(r.URL.Query().Get("id"))
	if !ok {
		errJSON(w, http.StatusNotFound, "action not found")
		return
	}
	_, protected := h.o.AppPWs[a.App]
	if protected {
		tok := r.Header.Get("X-Apidoc-App-Token")
		if data, ok := auth.Verify(h.o.Auth.Secret, tok); !ok || data != a.App {
			writeJSON(w, http.StatusUnauthorized, map[string]any{
				"code": http.StatusUnauthorized, "msg": "app token required", "app": a.App,
			})
			return
		}
	}
	var md bytes.Buffer
	_ = goldmark.Convert([]byte(a.Markdown), &md)
	writeJSON(w, http.StatusOK, map[string]any{"code": 0, "msg": "ok", "data": map[string]any{
		"id": a.ID, "app": a.App, "version": a.Version, "controller": a.Controller,
		"method": a.Method, "url": a.URL, "title": a.Title, "desc": a.Desc,
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
	if len(h.o.AppPWs) > 0 {
		if c, err := r.Cookie(cookieName); err != nil {
			errJSON(w, http.StatusUnauthorized, "login required")
			return
		} else if data, ok := auth.Verify(h.o.Auth.Secret, c.Value); !ok || data != "session" {
			errJSON(w, http.StatusUnauthorized, "login required")
			return
		}
	}
	p, err := h.o.Store.Project()
	if err != nil {
		errJSON(w, http.StatusInternalServerError, "export failed")
		return
	}
	switch r.URL.Query().Get("format") {
	case "", "json":
		writeJSON(w, http.StatusOK, map[string]any{"code": 0, "msg": "ok", "data": p})
	case "typescript":
		b, _ := json.Marshal(map[string]any{"code": 0, "msg": "ok", "data": export.Typescript(p)})
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(b)
	default:
		errJSON(w, http.StatusBadRequest, "unknown format")
	}
}

// actionMock returns the action's own mock string, or generated example
// values keyed by param name when none was registered.
func actionMock(a *model.Action) string {
	if a.Mock != "" {
		return a.Mock
	}
	return mock.Action(a)
}

// ui serves the embedded single-file UI.
func (h *apiHandler) ui(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(indexHTML)
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

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func errJSON(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]any{"code": status, "msg": msg})
}

// withAuth gates the /api endpoints when global auth is enabled.
// Login and app-login are the way in; the UI itself stays reachable so
// its login screen can render (it shows on 401 from /api/menus).
func withAuth(o Opts, next http.Handler) http.Handler {
	if !o.Auth.Enable {
		return next
	}
	open := map[string]bool{"/api/login": true, "/api/app-login": true}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api") || open[r.URL.Path] {
			next.ServeHTTP(w, r)
			return
		}
		if c, err := r.Cookie(cookieName); err == nil {
			// Only the "session" token may act as the session cookie; an
			// app-scoped token (weaker secret holder) must not bypass this gate.
			if data, ok := auth.Verify(o.Auth.Secret, c.Value); ok && data == "session" {
				next.ServeHTTP(w, r)
				return
			}
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
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Apidoc-App-Token")
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

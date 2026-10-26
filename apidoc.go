// Package apidoc is a universal Go API-documentation plugin library:
// docs are declared as typed structs riding along with route
// registration and served as a JSON API, with adapters for net/http,
// gin, echo, chi and fiber. Rendering and test requests are handled by
// an external UI consuming the API.
package apidoc

import (
	"fmt"
	"net/http"
	"sync"

	"github.com/erikwang2013/apidoc-go/adapter"
	"github.com/erikwang2013/apidoc-go/auth"
	"github.com/erikwang2013/apidoc-go/model"
	"github.com/erikwang2013/apidoc-go/server"
	"github.com/erikwang2013/apidoc-go/server/uiapi"
	"github.com/erikwang2013/apidoc-go/store"
)

// Config configures a Server. Zero values get sensible defaults.
type Config struct {
	Prefix       string // default "/apidoc"
	Title        string // default "API Docs"
	Desc         string
	GlobalParams []Param // merged into every action
	Auth         AuthConfig
	Apps         []AppConfig
	DebugOrigins []string // origins allowed CORS access (default none)
	// PublicHost is the fallback absolute base URL used by the external UI
	// for browser debugging. Empty means same-origin.
	PublicHost string
	// ShareDir stores UIC-1 share records; empty means a directory under
	// os.TempDir().
	ShareDir string
	// Generator and CodeTemplates are the declarative UIC-1 generator
	// descriptors; rendering is side-effect free.
	Generator     []GeneratorSpec
	CodeTemplates []CodeTemplateSpec
	// ShareActions declares the actions handleApiShareAction may return.
	ShareActions []ShareActionSpec
	// Debug enables server-side relay execution; the zero value fails closed.
	Debug DebugConfig
}

// UIC-1 management types; see server/uiapi.
type (
	GeneratorSpec    = uiapi.GeneratorSpec
	GeneratorFile    = uiapi.GeneratorFile
	CodeTemplateSpec = uiapi.CodeTemplateSpec
	ShareActionSpec  = uiapi.ShareActionSpec
	DebugConfig      = uiapi.DebugConfig
)

// AuthConfig is the auth configuration.
type AuthConfig = auth.Config

// AppConfig declares a doc app: Key is the app key used in Doc.App,
// Title is shown in the external UI, Password (optional) protects that
// app's detail data with a scoped token.
type AppConfig struct {
	Key, Title, Password string
	// PublicHost is this app's absolute debug base URL; falls back to Config.PublicHost.
	PublicHost string
}

// Server is one doc server instance. Create with New; register routes
// with Register; attach to a framework with Mount. No globals.
type Server struct {
	mu        sync.RWMutex
	cfg       Config
	st        *store.Store
	ad        adapter.Framework
	pending   []Route         // registrations made before Mount, replayed on Mount
	forwarded map[string]bool // "METHOD url" already handed to the framework
	appPw     map[string]string
	once      sync.Once
	handler   http.Handler
}

// New applies defaults and returns a ready Server.
func New(cfg Config) *Server {
	if cfg.Prefix == "" {
		cfg.Prefix = "/apidoc"
	}
	if cfg.Title == "" {
		cfg.Title = "API Docs"
	}
	s := &Server{cfg: cfg, st: store.New(cfg.Title), appPw: map[string]string{}, forwarded: map[string]bool{}}
	for _, a := range cfg.Apps {
		s.st.SetAppMeta(a.Key, a.Title)
		if a.Password != "" {
			s.appPw[a.Key] = a.Password
		}
	}
	return s
}

// Register records the doc and forwards the handler to the framework
// adapter (deferred to Mount if the adapter is not attached yet).
// Registering the same (app, version, method, url) again replaces the
// doc — later registration wins.
func (s *Server) Register(r Route) error {
	if r.Method == "" || r.URL == "" {
		return fmt.Errorf("apidoc: method and url are required")
	}
	r.Doc.Protocol = model.NormalizeProtocol(r.Doc.Protocol)
	if r.Doc.Protocol == "" {
		r.Doc.Protocol = model.ProtocolOf(r.URL)
	}
	if r.Doc.Protocol != "http" {
		s.st.Register(r.Method, r.URL, r.Doc)
		return nil
	}
	// Derive body params from the handler's struct args when the doc has none.
	if len(r.Doc.Params) == 0 && r.Handler != nil {
		if ps := reflectParams(r.Handler); len(ps) > 0 {
			r.Doc.Params = ps
		}
	}
	s.st.Register(r.Method, r.URL, r.Doc)
	s.mu.Lock()
	defer s.mu.Unlock()
	key := r.Method + " " + r.URL
	if s.forwarded[key] {
		// Doc replaced in the store; the framework already has this route,
		// and re-registering would conflict with itself.
		return nil
	}
	if s.ad == nil {
		s.pending = append(s.pending, r)
		return nil
	}
	s.forwarded[key] = true
	return s.ad.Register(r.Method, r.URL, r.Handler)
}

// Mount attaches the doc JSON API to the framework at the configured
// prefix, replaying any registrations made beforehand.
func (s *Server) Mount(a adapter.Framework) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ad = a
	p := s.pending
	s.pending = nil
	for _, r := range p {
		key := r.Method + " " + r.URL
		if s.forwarded[key] {
			continue
		}
		if err := a.Register(r.Method, r.URL, r.Handler); err != nil {
			return err
		}
		s.forwarded[key] = true
	}
	a.Mount(s.cfg.Prefix, s.Handler())
	return nil
}

// Handler returns the doc server's JSON API handler.
func (s *Server) Handler() http.Handler {
	s.once.Do(func() {
		appHosts := map[string]string{}
		for _, app := range s.cfg.Apps {
			if app.PublicHost != "" {
				appHosts[app.Key] = app.PublicHost
			}
		}
		s.handler = server.Handler(server.Opts{
			Prefix:        s.cfg.Prefix,
			Store:         s.st,
			Auth:          s.cfg.Auth,
			AppPWs:        s.appPw,
			DebugOrigins:  s.cfg.DebugOrigins,
			GlobalParams:  s.cfg.GlobalParams,
			PublicHost:    s.cfg.PublicHost,
			AppHosts:      appHosts,
			ShareDir:      s.cfg.ShareDir,
			Generator:     s.cfg.Generator,
			CodeTemplates: s.cfg.CodeTemplates,
			ShareActions:  s.cfg.ShareActions,
			Debug:         s.cfg.Debug,
		})
	})
	return s.handler
}

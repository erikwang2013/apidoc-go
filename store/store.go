// Package store is the in-memory doc store: thread-safe, JSON
// (de)serializable, deduplicated by (app, version, method, url).
package store

import (
	"encoding/base64"
	"encoding/json"
	"net/url"
	"sync"

	"github.com/erikwang2013/apidoc-go/model"
)

// Store keeps the doc tree and a flat index of actions by ID.
type Store struct {
	mu   sync.RWMutex
	proj *model.Project
	byID map[string]*model.Action
}

// New returns an empty store for the named project.
func New(name string) *Store {
	return &Store{proj: &model.Project{Name: name}, byID: map[string]*model.Action{}}
}

// ActionID builds the stable ID of (app, version, method, url).
// Base64 so the ID survives embedding in a query string raw (no
// percent-escapes, no slashes).
func ActionID(app, version, method, u string) string {
	key := url.PathEscape(app) + "/" + url.PathEscape(version) + "/" + url.PathEscape(method) + "/" + url.PathEscape(u)
	return base64.RawURLEncoding.EncodeToString([]byte(key))
}

// SetAppMeta creates or updates the app node's title.
func (s *Store) SetAppMeta(key, title string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if title != "" {
		s.app(key).Title = title
	}
}

// Register records (or replaces, if the same endpoint was already
// registered — later registration wins) an action.
func (s *Store) Register(method, u string, d model.Doc) {
	if d.App == "" {
		d.App = "default"
	}
	if d.Version == "" {
		d.Version = "v1"
	}
	if d.Controller == "" {
		d.Controller = "default"
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	id := ActionID(d.App, d.Version, method, u)
	a := &model.Action{
		ID: id, App: d.App, Version: d.Version, Controller: d.Controller,
		Method: method, URL: u, Title: d.Title, Desc: d.Desc, Author: d.Author,
		Params: d.Params, Responses: d.Responses, Markdown: d.Markdown,
		Mock: d.Mock, Sort: d.Sort,
	}
	if old, ok := s.byID[id]; ok {
		*old = *a // keep tree position, replace content
		return
	}
	ctl := s.controller(s.version(s.app(d.App), d.Version), d.Controller)
	s.byID[id] = a
	ctl.Actions = append(ctl.Actions, a)
}

// clone deep-copies v via a JSON round-trip (model types are JSON-shaped).
// The copy happens while the caller holds the read lock so concurrent
// Register writes can't race the marshal.
func clone[T any](v T) (T, error) {
	b, err := json.Marshal(v)
	if err != nil {
		var zero T
		return zero, err
	}
	var out T
	err = json.Unmarshal(b, &out)
	return out, err
}

// Menus returns a deep copy of the app tree (id/title/sort per action).
func (s *Store) Menus() []*model.App {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out, err := clone(s.proj.Apps)
	if err != nil {
		return nil
	}
	return out
}

// Project returns a deep copy of the whole project tree.
func (s *Store) Project() (*model.Project, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return clone(s.proj)
}

// Action returns a deep copy of the action with the given ID.
func (s *Store) Action(id string) (*model.Action, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	a := s.byID[id]
	if a == nil {
		return nil, false
	}
	out, err := clone(a)
	if err != nil {
		return nil, false
	}
	return out, true
}

// MarshalJSON serializes the whole store (doc data only).
func (s *Store) MarshalJSON() ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.proj == nil {
		return json.Marshal(&model.Project{})
	}
	return json.Marshal(s.proj)
}

// UnmarshalJSON loads a store and rebuilds the action index.
func (s *Store) UnmarshalJSON(b []byte) error {
	var p model.Project
	if err := json.Unmarshal(b, &p); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.proj = &p
	s.byID = map[string]*model.Action{}
	for _, app := range p.Apps {
		for _, v := range app.Versions {
			for _, c := range v.Controllers {
				for _, a := range c.Actions {
					s.byID[a.ID] = a
				}
			}
		}
	}
	return nil
}

func (s *Store) app(key string) *model.App {
	for _, a := range s.proj.Apps {
		if a.Key == key {
			return a
		}
	}
	a := &model.App{Key: key, Title: key}
	s.proj.Apps = append(s.proj.Apps, a)
	return a
}

func (s *Store) version(app *model.App, name string) *model.Version {
	for _, v := range app.Versions {
		if v.Name == name {
			return v
		}
	}
	v := &model.Version{Name: name}
	app.Versions = append(app.Versions, v)
	return v
}

func (s *Store) controller(v *model.Version, name string) *model.Controller {
	for _, c := range v.Controllers {
		if c.Name == name {
			return c
		}
	}
	c := &model.Controller{Name: name}
	v.Controllers = append(v.Controllers, c)
	return c
}

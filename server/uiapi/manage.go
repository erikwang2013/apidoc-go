package uiapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"sort"
	"text/template"

	"github.com/erikwang2013/apidoc-go/model"
)

// GeneratorSpec is one declarative UIC-1 generator: form descriptors, file
// templates and an optional table descriptor. Rendering never touches disk or
// a database.
type GeneratorSpec struct {
	Name  string          `json:"name"`
	Title string          `json:"title"`
	Files []GeneratorFile `json:"files"`
	Form  map[string]any  `json:"form,omitempty"`
	Table map[string]any  `json:"table,omitempty"`
}

// GeneratorFile is one rendered output. Template is the text/template source
// exposed in /ui/v1/config; Path may be overridden by the request.
type GeneratorFile struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	Namespace string `json:"namespace,omitempty"`
	Template  string `json:"template,omitempty"`
	Rules     any    `json:"rules,omitempty"`
}

// CodeTemplateSpec is one declarative code template. Content is the
// text/template source and is never exposed through /ui/v1/config.
type CodeTemplateSpec struct {
	Title      string         `json:"title"`
	Template   string         `json:"template"`
	Content    string         `json:"-"`
	SelectMode string         `json:"select_mode,omitempty"`
	Multiple   bool           `json:"multiple,omitempty"`
	Language   string         `json:"language,omitempty"`
	Tips       string         `json:"tips,omitempty"`
	Limit      int            `json:"limit,omitempty"`
	Form       map[string]any `json:"form,omitempty"`
}

// ShareActionSpec declares one share action. handleApiShareAction returns
// {action, params} for the UI to interpret; no server-side code runs.
type ShareActionSpec struct {
	Name   string         `json:"name"`
	Action string         `json:"action,omitempty"`
	Params map[string]any `json:"params,omitempty"`
}

func renderTemplate(name, source string, data any) (string, error) {
	t, err := template.New(name).Parse(source)
	if err != nil {
		return "", err
	}
	var out bytes.Buffer
	if err := t.Execute(&out, data); err != nil {
		return "", err
	}
	return out.String(), nil
}

func (h *handler) cacheClear(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) {
		return
	}
	if h.o.CacheClear == nil {
		writeError(w, http.StatusNotImplemented, 501, "cache management unavailable")
		return
	}
	h.o.CacheClear()
	writeJSON(w, http.StatusOK, ok(map[string]any{"ok": true, "mode": "server"}))
}

func (h *handler) cacheWarm(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) {
		return
	}
	if h.o.CacheWarm == nil {
		writeError(w, http.StatusNotImplemented, 501, "cache management unavailable")
		return
	}
	h.o.CacheWarm()
	writeJSON(w, http.StatusOK, ok(map[string]any{"ok": true, "mode": "server", "message": "cache rebuilt"}))
}

func (h *handler) generator(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) {
		return
	}
	body, err := decode[struct {
		Index  int              `json:"index"`
		Form   map[string]any   `json:"form"`
		Files  []map[string]any `json:"files"`
		Tables any              `json:"tables"`
	}](r)
	if err != nil {
		writeError(w, http.StatusBadRequest, 400, "invalid body")
		return
	}
	if body.Index < 0 || body.Index >= len(h.o.Generator) {
		writeError(w, http.StatusNotFound, 404, "generator not found")
		return
	}
	spec := h.o.Generator[body.Index]
	paths := map[string]string{}
	for _, file := range body.Files {
		if name, _ := file["name"].(string); name != "" {
			if path, _ := file["path"].(string); path != "" {
				paths[name] = path
			}
		}
	}
	data := templateData(body.Form, map[string]any{
		"files": body.Files, "tables": body.Tables,
	})
	files := make([]map[string]any, 0, len(spec.Files))
	for _, file := range spec.Files {
		content, err := renderTemplate(file.Name, file.Template, data)
		if err != nil {
			writeError(w, http.StatusInternalServerError, 500, "template error")
			return
		}
		path := file.Path
		if override := paths[file.Name]; override != "" {
			path = override
		}
		files = append(files, map[string]any{"name": file.Name, "path": path, "content": content})
	}
	writeJSON(w, http.StatusOK, ok(map[string]any{"files": files}))
}

// codeTemplateRef accepts both the UI's template object and a bare template
// name (the contract fixture uses the latter).
type codeTemplateRef struct {
	Title    string
	Template string
}

func (c *codeTemplateRef) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) > 0 && b[0] == '"' {
		return json.Unmarshal(b, &c.Template)
	}
	var v struct {
		Title    string `json:"title"`
		Template string `json:"template"`
	}
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	c.Title, c.Template = v.Title, v.Template
	return nil
}

func (h *handler) renderCodeTemplate(w http.ResponseWriter, r *http.Request) {
	body, err := decode[struct {
		AppKey   string          `json:"appKey"`
		Template codeTemplateRef `json:"template"`
		Selected []string        `json:"selected"`
		Form     map[string]any  `json:"form"`
	}](r)
	if err != nil {
		writeError(w, http.StatusBadRequest, 400, "invalid body")
		return
	}
	if _, allowed := h.authorize(w, r, body.AppKey, ""); !allowed {
		return
	}
	var spec *CodeTemplateSpec
	for i := range h.o.CodeTemplates {
		candidate := &h.o.CodeTemplates[i]
		if (body.Template.Template != "" && candidate.Template == body.Template.Template) ||
			(body.Template.Template == "" && body.Template.Title != "" && candidate.Title == body.Template.Title) {
			spec = candidate
			break
		}
	}
	if spec == nil {
		writeError(w, http.StatusNotFound, 404, "template not found")
		return
	}
	apis := []map[string]any{}
	for _, id := range body.Selected {
		action, found := h.o.Store.Action(id)
		if !found || !actionInApp(action, body.AppKey) {
			continue
		}
		apis = append(apis, h.detailData(action))
	}
	data := templateData(body.Form, map[string]any{
		"appKey": body.AppKey, "selected": body.Selected, "apis": apis,
	})
	code, err := renderTemplate(spec.Title, spec.Content, data)
	if err != nil {
		writeError(w, http.StatusInternalServerError, 500, "template error")
		return
	}
	writeJSON(w, http.StatusOK, ok(map[string]any{"code": code}))
}

// templateData keeps the named context keys and also promotes form fields so
// templates can use {{.name}} or {{.form.name}}.
func templateData(form map[string]any, named map[string]any) map[string]any {
	data := map[string]any{"form": form}
	for key, value := range named {
		data[key] = value
	}
	for key, value := range form {
		if _, exists := data[key]; !exists {
			data[key] = value
		}
	}
	return data
}

// actionInApp reports whether action belongs to appKey (canonical app|version,
// bare app, or empty = any).
func actionInApp(action *model.Action, appKey string) bool {
	if appKey == "" {
		return true
	}
	name, version := splitAppKey(appKey)
	return (name == "" || action.App == name) && (version == "" || action.Version == version)
}

func (h *handler) exportAllData(w http.ResponseWriter, r *http.Request) {
	body, err := decode[struct {
		Lang   string `json:"lang"`
		Key    string `json:"key"`
		AppKey string `json:"appKey"`
	}](r)
	if err != nil {
		writeError(w, http.StatusBadRequest, 400, "invalid body")
		return
	}
	var rec *shareRecord
	switch {
	case body.Key != "":
		if !h.requireAdmin(w, r) {
			return
		}
		rec, _ = h.shares.get(body.Key)
		if rec == nil {
			writeError(w, http.StatusNotFound, 404, "share not exists")
			return
		}
	case body.AppKey != "":
		var allowed bool
		rec, allowed = h.authorize(w, r, body.AppKey, "")
		if !allowed {
			return
		}
	default:
		if !h.requireAdmin(w, r) {
			return
		}
	}
	project, err := h.o.Store.Project()
	if err != nil {
		writeError(w, http.StatusInternalServerError, 500, "project unavailable")
		return
	}
	apiData := []map[string]any{}
	for _, app := range project.Apps {
		for _, version := range app.Versions {
			canonical := canonicalAppKey(app.Key, version.Name)
			if body.AppKey != "" && !matchesAppKey(app.Key, version.Name, body.AppKey) {
				continue
			}
			if rec != nil && !h.shareAllowsApp(rec, canonical) {
				continue
			}
			controllers := []map[string]any{}
			for _, ctl := range version.Controllers {
				actions := append([]*model.Action{}, ctl.Actions...)
				sort.SliceStable(actions, func(i, j int) bool { return actions[i].Sort < actions[j].Sort })
				children := []map[string]any{}
				for _, action := range actions {
					if !rec.apiAllowed(action.ID) {
						continue
					}
					children = append(children, h.detailData(action))
				}
				if len(children) == 0 {
					continue
				}
				controllers = append(controllers, map[string]any{
					"name": ctl.Name, "title": ctl.Name, "menuKey": "controller:" + ctl.Name,
					"controller": ctl.Name, "path": ctl.Name, "appKey": canonical,
					"children": children,
				})
			}
			if rec != nil && len(controllers) == 0 {
				continue
			}
			title := app.Title
			if len(app.Versions) > 1 {
				title += " / " + version.Name
			}
			apiData = append(apiData, map[string]any{
				"key": app.Key, "title": title,
				"appKey": canonical, "menuKey": canonical, "children": controllers,
			})
		}
	}
	writeJSON(w, http.StatusOK, ok(map[string]any{"apiData": apiData}))
}

func matchesAppKey(app, version, key string) bool {
	name, wanted := splitAppKey(key)
	return (name == "" || name == app) && (wanted == "" || wanted == version)
}

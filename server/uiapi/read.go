package uiapi

import (
	"net/http"
	"sort"
	"strings"

	"github.com/erikwang2013/apidoc-go/model"
	"github.com/erikwang2013/apidoc-go/server/uiapi/debug"
)

func splitAppKey(key string) (string, string) {
	key = strings.ReplaceAll(key, "%7C", "|")
	if i := strings.IndexByte(key, '|'); i >= 0 {
		return key[:i], key[i+1:]
	}
	return key, ""
}

func canonicalAppKey(app, version string) string { return app + "|" + version }

func (h *handler) appLeaf(app *model.App, version *model.Version) map[string]any {
	key := canonicalAppKey(app.Key, version.Name)
	host := h.o.AppHosts[app.Key]
	if host == "" {
		host = h.o.PublicHost
	}
	title := app.Title
	if len(app.Versions) > 1 {
		title = app.Title + " / " + version.Name
	}
	return map[string]any{
		"key":         key,
		"title":       title,
		"path":        key,
		"host":        host,
		"hasPassword": h.o.AppPWs[app.Key] != "",
		"groups":      []any{},
	}
}

func (h *handler) findAppVersion(project *model.Project, key string) (*model.App, *model.Version, string, bool) {
	appName, versionName := splitAppKey(key)
	var fallbackApp *model.App
	for _, app := range project.Apps {
		if appName != "" && app.Key != appName {
			continue
		}
		if fallbackApp == nil {
			fallbackApp = app
		}
		for _, version := range app.Versions {
			if versionName != "" && version.Name != versionName {
				continue
			}
			return app, version, canonicalAppKey(app.Key, version.Name), true
		}
	}
	if fallbackApp != nil && len(fallbackApp.Versions) > 0 {
		version := fallbackApp.Versions[0]
		return fallbackApp, version, canonicalAppKey(fallbackApp.Key, version.Name), true
	}
	return nil, nil, "", false
}

func (h *handler) globalParams() map[string]any {
	out := map[string]any{
		"header": []map[string]any{},
		"query":  []map[string]any{},
		"body":   []map[string]any{},
	}
	for _, p := range h.o.GlobalParams {
		bucket := "query"
		switch strings.ToLower(p.In) {
		case "header":
			bucket = "header"
		case "body", "payload":
			bucket = "body"
		}
		out[bucket] = append(out[bucket].([]map[string]any), paramItem(p))
	}
	return out
}

func (h *handler) config(w http.ResponseWriter, r *http.Request) {
	appKey := r.URL.Query().Get("appKey")
	shareKey := r.URL.Query().Get("shareKey")
	rec, allowed := h.authorize(w, r, appKey, shareKey)
	if !allowed {
		return
	}
	project, err := h.o.Store.Project()
	if err != nil {
		writeError(w, http.StatusInternalServerError, 500, "project unavailable")
		return
	}
	apps := []map[string]any{}
	for _, app := range project.Apps {
		for _, version := range app.Versions {
			if rec != nil && !h.shareAllowsApp(rec, canonicalAppKey(app.Key, version.Name)) {
				continue
			}
			apps = append(apps, h.appLeaf(app, version))
		}
	}
	generator := h.o.Generator
	if generator == nil {
		generator = []GeneratorSpec{}
	}
	codeTemplates := h.o.CodeTemplates
	if codeTemplates == nil {
		codeTemplates = []CodeTemplateSpec{}
	}
	actions := h.o.ShareActions
	if actions == nil {
		actions = []ShareActionSpec{}
	}
	writeJSON(w, http.StatusOK, ok(map[string]any{
		"title":         project.Name,
		"desc":          "",
		"apps":          apps,
		"cache":         map[string]any{"enable": h.cacheManageable()},
		"params":        h.globalParams(),
		"responses":     map[string]any{"success": []any{}, "error": []any{}},
		"generator":     generator,
		"code_template": codeTemplates,
		"share":         map[string]any{"enable": true, "actions": actions},
		"export_config": map[string]any{"enable": true},
		"capabilities":  h.capabilitiesData(),
	}))
}

func (h *handler) menus(w http.ResponseWriter, r *http.Request) {
	body, err := decode[struct {
		AppKey   string `json:"appKey"`
		ShareKey string `json:"shareKey"`
	}](r)
	if err != nil {
		writeError(w, http.StatusBadRequest, 400, "invalid body")
		return
	}
	if !requireAppKey(w, body.AppKey) {
		return
	}
	rec, allowed := h.authorize(w, r, body.AppKey, body.ShareKey)
	if !allowed {
		return
	}
	project, err := h.o.Store.Project()
	if err != nil {
		writeError(w, http.StatusInternalServerError, 500, "project unavailable")
		return
	}
	app, version, key, found := h.findAppVersion(project, body.AppKey)
	if !found {
		writeError(w, http.StatusNotFound, 404, "app not found")
		return
	}
	data := []map[string]any{}
	for _, ctl := range version.Controllers {
		actions := append([]*model.Action{}, ctl.Actions...)
		sort.SliceStable(actions, func(i, j int) bool { return actions[i].Sort < actions[j].Sort })
		children := []map[string]any{}
		for _, action := range actions {
			if !rec.apiAllowed(action.ID) {
				continue
			}
			url := action.URL
			if url == "" {
				url = channelAddress(action)
			}
			item := map[string]any{
				"name":       action.Title,
				"title":      action.Title,
				"menuKey":    action.ID,
				"method":     action.Method,
				"url":        url,
				"controller": action.Controller,
				"author":     action.Author,
				"notDebug":   false,
			}
			if action.Protocol != "" && action.Protocol != "http" {
				item["protocolKey"] = action.Protocol
				item["protocolLabel"] = protocolLabel(action.Protocol)
				item["channelAddress"] = channelAddress(action)
			}
			children = append(children, item)
		}
		if rec != nil && rec.Type == "api" && len(children) == 0 {
			continue
		}
		data = append(data, map[string]any{
			"name":       ctl.Name,
			"title":      ctl.Name,
			"menuKey":    "controller:" + ctl.Name,
			"controller": ctl.Name,
			"path":       ctl.Name,
			"appKey":     key,
			"children":   children,
		})
	}
	writeJSON(w, http.StatusOK, ok(map[string]any{
		"app":    h.appLeaf(app, version),
		"data":   data,
		"tags":   []string{},
		"groups": []any{},
	}))
}

func (h *handler) detail(w http.ResponseWriter, r *http.Request) {
	body, err := decode[struct {
		AppKey   string `json:"appKey"`
		Path     string `json:"path"`
		ShareKey string `json:"shareKey"`
	}](r)
	if err != nil {
		writeError(w, http.StatusBadRequest, 400, "invalid body")
		return
	}
	action, found := h.o.Store.Action(body.Path)
	if !found {
		writeError(w, http.StatusNotFound, 404, "action not found")
		return
	}
	if body.AppKey != "" && canonicalAppKey(action.App, action.Version) != body.AppKey {
		writeError(w, http.StatusNotFound, 404, "action not found")
		return
	}
	rec, allowed := h.authorize(w, r, canonicalAppKey(action.App, action.Version), body.ShareKey)
	if !allowed {
		return
	}
	if !rec.apiAllowed(action.ID) {
		writeError(w, http.StatusForbidden, 403, "share api not allowed")
		return
	}
	writeJSON(w, http.StatusOK, ok(h.detailData(action)))
}

func (h *handler) detailData(action *model.Action) map[string]any {
	header := []map[string]any{}
	query := []map[string]any{}
	route := []map[string]any{}
	params := []map[string]any{}
	for _, p := range mergeParams(h.o.GlobalParams, action.Params) {
		item := paramItem(p)
		switch strings.ToLower(p.In) {
		case "header":
			header = append(header, item)
		case "query":
			query = append(query, item)
		case "path", "route":
			route = append(route, item)
		default:
			params = append(params, item)
		}
	}
	success, failure := responseItems(action.Responses)
	paramType := "json"
	contentType := "application/json"
	for _, p := range params {
		ty, _ := p["type"].(string)
		if ty == "file" || ty == "files" {
			paramType = "formdata"
			contentType = "multipart/form-data"
			break
		}
	}
	protocol := strings.TrimSpace(action.Protocol)
	if protocol == "" {
		protocol = "http"
	}
	httpProtocol := isHTTPProtocol(protocol)
	mode, debuggable, reason := h.protocolMode(protocol, action.Method)
	address := channelAddress(action)
	fields, defaults := debugFields(action, address, httpProtocol)
	data := map[string]any{
		"name":            action.Title,
		"title":           action.Title,
		"menuKey":         action.ID,
		"method":          action.Method,
		"url":             action.URL,
		"appKey":          canonicalAppKey(action.App, action.Version),
		"author":          action.Author,
		"tag":             []string{},
		"desc":            action.Desc,
		"md":              action.Markdown,
		"contentType":     contentType,
		"paramType":       paramType,
		"notDebug":        false,
		"header":          header,
		"query":           query,
		"routeParam":      route,
		"param":           params,
		"responseSuccess": success,
		"responseError":   failure,
		"responseStatus":  []any{},
		"protocol": map[string]any{
			"name":        protocol,
			"label":       protocolLabel(protocol),
			"transport":   protocolTransport(protocol),
			"mode":        mode,
			"http":        httpProtocol,
			"debuggable":  debuggable,
			"debugReason": reason,
		},
		"channel": map[string]any{
			"address": address,
			"action":  action.Method,
			"meta":    action.ProtocolOpts,
		},
		"debug": map[string]any{
			"mode":             mode,
			"driver":           protocol,
			"relayEnabled":     strings.HasPrefix(mode, "relay-") && debuggable,
			"debuggable":       debuggable,
			"debugReason":      reason,
			"fields":           fields,
			"defaults":         defaults,
			"meta":             map[string]any{},
			"responseEncoding": responseEncoding(protocol, httpProtocol),
			"hints":            "",
		},
		"definitions": definitions(action),
		"validation":  okValidation(),
	}
	if action.Mock != "" {
		data["mock"] = action.Mock
	}
	return data
}

func (h *handler) docMenus(w http.ResponseWriter, r *http.Request) {
	appKey := r.URL.Query().Get("appKey")
	shareKey := r.URL.Query().Get("shareKey")
	if r.Method == http.MethodPost {
		if body, err := decode[struct {
			AppKey   string `json:"appKey"`
			ShareKey string `json:"shareKey"`
		}](r); err == nil {
			appKey = body.AppKey
			shareKey = body.ShareKey
		}
	}
	if !requireAppKey(w, appKey) {
		return
	}
	rec, allowed := h.authorize(w, r, appKey, shareKey)
	if !allowed {
		return
	}
	project, err := h.o.Store.Project()
	if err != nil {
		writeError(w, http.StatusInternalServerError, 500, "project unavailable")
		return
	}
	out := []map[string]any{}
	for _, app := range project.Apps {
		for _, version := range app.Versions {
			if appKey != "" && canonicalAppKey(app.Key, version.Name) != appKey {
				continue
			}
			for _, ctl := range version.Controllers {
				children := []map[string]any{}
				for _, action := range ctl.Actions {
					if action.Markdown == "" || !rec.apiAllowed(action.ID) {
						continue
					}
					children = append(children, map[string]any{
						"name":    action.Title,
						"title":   action.Title,
						"menuKey": "doc:" + action.ID,
						"path":    action.ID,
					})
				}
				if len(children) > 0 {
					out = append(out, map[string]any{
						"name":     ctl.Name,
						"title":    ctl.Name,
						"menuKey":  "doc-controller:" + canonicalAppKey(app.Key, version.Name) + ":" + ctl.Name,
						"children": children,
					})
				}
			}
		}
	}
	writeJSON(w, http.StatusOK, ok(out))
}

func (h *handler) docDetail(w http.ResponseWriter, r *http.Request) {
	body, err := decode[struct {
		AppKey   string `json:"appKey"`
		Path     string `json:"path"`
		ShareKey string `json:"shareKey"`
	}](r)
	if err != nil {
		writeError(w, http.StatusBadRequest, 400, "invalid body")
		return
	}
	if !requireAppKey(w, body.AppKey) {
		return
	}
	action, found := h.o.Store.Action(body.Path)
	if !found || action.Markdown == "" {
		writeError(w, http.StatusNotFound, 404, "doc not found")
		return
	}
	if body.AppKey != "" && canonicalAppKey(action.App, action.Version) != body.AppKey {
		writeError(w, http.StatusNotFound, 404, "doc not found")
		return
	}
	rec, allowed := h.authorize(w, r, canonicalAppKey(action.App, action.Version), body.ShareKey)
	if !allowed {
		return
	}
	if !rec.apiAllowed(action.ID) {
		writeError(w, http.StatusForbidden, 403, "share api not allowed")
		return
	}
	writeJSON(w, http.StatusOK, ok(map[string]any{"content": action.Markdown}))
}

func mergeParams(global, own []model.Param) []model.Param {
	index := map[string]int{}
	out := make([]model.Param, 0, len(global)+len(own))
	for _, p := range global {
		index[p.Name+"\x00"+p.In] = len(out)
		out = append(out, p)
	}
	for _, p := range own {
		key := p.Name + "\x00" + p.In
		if i, exists := index[key]; exists {
			out[i] = p
		} else {
			out = append(out, p)
		}
	}
	return out
}

func paramItem(p model.Param) map[string]any {
	item := map[string]any{
		"name":    p.Name,
		"type":    p.Type,
		"desc":    p.Desc,
		"require": p.Required,
	}
	if p.Default != "" {
		item["default"] = p.Default
	}
	if p.Mock != "" {
		item["mock"] = p.Mock
	}
	if len(p.Children) > 0 {
		children := make([]map[string]any, 0, len(p.Children))
		for _, child := range p.Children {
			children = append(children, paramItem(child))
		}
		item["children"] = children
	}
	return item
}

func responseItems(responses []model.Response) ([]map[string]any, []map[string]any) {
	success := []map[string]any{}
	failure := []map[string]any{}
	for _, response := range responses {
		items := []map[string]any{}
		if len(response.Fields) > 0 {
			for _, field := range response.Fields {
				items = append(items, paramItem(field))
			}
		} else {
			item := map[string]any{
				"name": response.Name, "type": response.Type,
				"desc": response.Desc, "require": true,
			}
			if response.Mock != "" {
				item["mock"] = response.Mock
			}
			items = append(items, item)
		}
		lower := strings.ToLower(response.Name)
		if strings.Contains(lower, "error") || strings.Contains(lower, "fail") ||
			strings.HasPrefix(response.Name, "4") || strings.HasPrefix(response.Name, "5") {
			failure = append(failure, items...)
		} else {
			success = append(success, items...)
		}
	}
	return success, failure
}

func isHTTPProtocol(protocol string) bool {
	switch protocol {
	case "", "http", "http2", "http3", "hls":
		return true
	}
	return false
}

func protocolLabel(protocol string) string {
	if info, found := model.ProtocolInfoOf(protocol); found {
		return info.Name
	}
	return strings.ToUpper(protocol)
}

func protocolTransport(protocol string) string {
	switch protocol {
	case "", "http", "http2", "http3", "hls", "soap", "graphql", "jsonrpc", "xmlrpc", "odata", "mcp", "a2a", "webhook":
		return "http"
	case "ws", "websocket":
		return "ws"
	case "sse":
		return "sse"
	default:
		return "tcp"
	}
}

// protocolMode advertises a relay mode only when this build has the executor
// and the deployment enabled server-side debug.
func (h *handler) protocolMode(protocol, method string) (string, bool, string) {
	switch protocol {
	case "", "http", "http2", "http3":
		// HTTP ids stay browser modes (Rust/plan parity); tagged builds also
		// expose http3 through the relay capability.
		return "browser-http", true, ""
	case "hls":
		return "browser-http", true, ""
	case "ws", "websocket":
		return "browser-websocket", true, ""
	case "sse":
		return "browser-sse", true, ""
	}
	kind := protocol
	if kind == "rawsocket" {
		kind = "raw"
	}
	hasUnary := debug.HasExecutor(kind)
	hasStream := debug.HasStream(kind)
	if !hasUnary && !hasStream {
		return "relay-unary", false, "server executor for `" + protocol + "` is not compiled in this build"
	}
	// Messaging ids default to a stream session; publish/send actions stay
	// unary like PHP, and kafka/xmpp are stream per the 3G.4 plan.
	mode := "relay-unary"
	switch kind {
	case "mqtt", "stomp", "amqp":
		action := strings.ToLower(method)
		if action != "publish" && action != "send" {
			mode = "relay-stream"
		}
	case "kafka", "xmpp":
		mode = "relay-stream"
	}
	if mode == "relay-unary" && !hasUnary {
		mode = "relay-stream"
	}
	if mode == "relay-stream" && !hasStream {
		mode = "relay-unary"
	}
	if !h.o.Debug.EnableServerSide {
		return mode, false, "server-side debug is disabled"
	}
	return mode, true, ""
}

// responseEncoding matches Rust/PHP: HTTP-compatible relay ids decode JSON
// even though they stay relay-unary because they execute via debugSend.
func responseEncoding(protocol string, httpProtocol bool) string {
	if httpProtocol {
		return "json"
	}
	switch protocol {
	case "soap", "graphql", "jsonrpc", "xmlrpc", "odata", "mcp", "a2a", "webhook":
		return "json"
	}
	return "text"
}

func channelAddress(action *model.Action) string {
	if value := action.ProtocolOpts["target"]; value != "" {
		return value
	}
	return action.URL
}

func debugFields(action *model.Action, address string, httpProtocol bool) ([]map[string]any, map[string]any) {
	fields := []map[string]any{}
	defaults := map[string]any{}
	if httpProtocol {
		return fields, defaults
	}
	fields = append(fields, map[string]any{
		"name": "target", "type": "string", "in": "target", "require": true, "default": address,
	})
	defaults["target"] = address
	keys := make([]string, 0, len(action.ProtocolOpts))
	for key := range action.ProtocolOpts {
		if key != "target" {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	for _, key := range keys {
		value := action.ProtocolOpts[key]
		fields = append(fields, map[string]any{
			"name": key, "type": "string", "in": "meta", "default": value,
		})
		defaults[key] = value
	}
	return fields, defaults
}

func definitions(action *model.Action) []map[string]any {
	if len(action.Events) == 0 {
		return []map[string]any{}
	}
	messages := make([]map[string]any, 0, len(action.Events))
	for _, event := range action.Events {
		direction := "request"
		if event.Direction == "in" {
			direction = "response"
		}
		message := map[string]any{
			"name":      event.Name,
			"direction": direction,
			"fields": []map[string]any{{
				"name": "body", "type": event.Type, "desc": event.Desc, "require": true,
			}},
			"sampleValidation": okValidation(),
		}
		if event.Mock != "" {
			message["sample"] = event.Mock
		}
		messages = append(messages, message)
	}
	return []map[string]any{{
		"format":     "asyncapi",
		"source":     "inline",
		"operation":  action.Method,
		"meta":       map[string]any{"protocol": action.Protocol},
		"messages":   messages,
		"validation": okValidation(),
	}}
}

func okValidation() map[string]any {
	return map[string]any{"ok": true, "errors": []any{}, "warnings": []any{}}
}

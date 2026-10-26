package uiapi

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"sync"
	"time"

	"github.com/erikwang2013/apidoc-go/auth"
)

// sharePageSize mirrors the PHP backend's share list page size.
const sharePageSize = 5

// shareKeyRe keeps record keys usable as file names (no path traversal).
var shareKeyRe = regexp.MustCompile(`^[a-zA-Z0-9-]{1,64}$`)

// shareRecord is one persisted share. Secret and PasswordHash never leave the
// store: API responses use shareView instead.
type shareRecord struct {
	Key          string   `json:"key"`
	Name         string   `json:"name"`
	Type         string   `json:"type"`
	AppKeys      []string `json:"appKeys,omitempty"`
	APIKeys      []string `json:"apiKeys,omitempty"`
	PasswordHash string   `json:"password_hash,omitempty"`
	Secret       string   `json:"secret"`
	CreateAt     string   `json:"create_at"`
}

// shareStore persists one JSON file per share: atomic write + rename, one
// mutex for the directory.
type shareStore struct {
	mu  sync.Mutex
	dir string
}

func (s *shareStore) file(key string) (string, bool) {
	if !shareKeyRe.MatchString(key) {
		return "", false
	}
	return filepath.Join(s.dir, key+".json"), true
}

func (s *shareStore) get(key string) (*shareRecord, bool) {
	path, ok := s.file(key)
	if !ok {
		return nil, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	var rec shareRecord
	if err := json.Unmarshal(b, &rec); err != nil || rec.Key != key {
		return nil, false
	}
	return &rec, true
}

func (s *shareStore) put(rec *shareRecord) error {
	path, ok := s.file(rec.Key)
	if !ok {
		return os.ErrInvalid
	}
	b, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return err
	}
	f, err := os.CreateTemp(s.dir, ".share-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp) // no-op after a successful rename
	if _, err := f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func (s *shareStore) delete(key string) bool {
	path, ok := s.file(key)
	if !ok {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	err := os.Remove(path)
	return err == nil || os.IsNotExist(err)
}

func (s *shareStore) list() []*shareRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil
	}
	out := make([]*shareRecord, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || filepath.Ext(name) != ".json" {
			continue
		}
		b, err := os.ReadFile(filepath.Join(s.dir, name))
		if err != nil {
			continue
		}
		var rec shareRecord
		if err := json.Unmarshal(b, &rec); err != nil || rec.Key == "" {
			continue
		}
		out = append(out, &rec)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CreateAt != out[j].CreateAt {
			return out[i].CreateAt > out[j].CreateAt
		}
		return out[i].Key < out[j].Key
	})
	return out
}

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func sharePasswordHash(secret, password string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(password))
	return hex.EncodeToString(mac.Sum(nil))
}

func sharePasswordOK(rec *shareRecord, password string) bool {
	if rec.PasswordHash == "" {
		return true
	}
	got := sharePasswordHash(rec.Secret, password)
	return subtle.ConstantTimeCompare([]byte(got), []byte(rec.PasswordHash)) == 1
}

// shareView is the record minus its credentials.
func shareView(rec *shareRecord) map[string]any {
	view := map[string]any{
		"key": rec.Key, "name": rec.Name, "type": rec.Type,
		"create_at": rec.CreateAt, "create_time": rec.CreateAt,
	}
	if len(rec.AppKeys) > 0 {
		view["appKeys"] = rec.AppKeys
	}
	if len(rec.APIKeys) > 0 {
		view["apiKeys"] = rec.APIKeys
	}
	return view
}

// shareAuth loads the share named by shareKey and, when it is password
// protected, requires a token signed with that share's own secret. A missing
// share is a 404; a bad/absent token is a 401.
func (h *handler) shareAuth(w http.ResponseWriter, r *http.Request, shareKey string) (*shareRecord, bool) {
	rec, found := h.shares.get(shareKey)
	if !found {
		writeError(w, http.StatusNotFound, 404, "share not exists")
		return nil, false
	}
	if rec.PasswordHash != "" {
		scope, ok := auth.Verify(rec.Secret, bearerToken(r))
		if !ok || scope != "share:"+rec.Key {
			writeError(w, http.StatusUnauthorized, 401, "token error")
			return nil, false
		}
	}
	return rec, true
}

// authorize gates a UIC-1 read. With a shareKey the share (and its optional
// password token) is the credential; without one the normal app/global token
// rules apply.
func (h *handler) authorize(w http.ResponseWriter, r *http.Request, appKey, shareKey string) (*shareRecord, bool) {
	if shareKey == "" {
		if !h.canAccess(r, appKey) {
			writeError(w, http.StatusUnauthorized, 401, "token error")
			return nil, false
		}
		return nil, true
	}
	rec, ok := h.shareAuth(w, r, shareKey)
	if !ok {
		return nil, false
	}
	if appKey != "" && !h.shareAllowsApp(rec, appKey) {
		writeError(w, http.StatusNotFound, 404, "share not exists")
		return nil, false
	}
	return rec, true
}

// shareAllowsApp reports whether the app (canonical app|version, or bare app)
// is inside the share scope. API shares without explicit appKeys derive their
// app scope from the selected API IDs.
func (h *handler) shareAllowsApp(rec *shareRecord, appKey string) bool {
	if rec.Type == "all" {
		return true
	}
	name, _ := splitAppKey(appKey)
	for _, key := range rec.AppKeys {
		if key == appKey || key == name {
			return true
		}
	}
	if rec.Type == "api" && len(rec.AppKeys) == 0 {
		for _, id := range rec.APIKeys {
			if action, ok := h.o.Store.Action(id); ok && canonicalAppKey(action.App, action.Version) == appKey {
				return true
			}
		}
	}
	return false
}

func (rec *shareRecord) apiAllowed(id string) bool {
	if rec == nil || rec.Type != "api" {
		return true
	}
	for _, key := range rec.APIKeys {
		if key == id {
			return true
		}
	}
	return false
}

// requireAdmin gates management endpoints: global auth on means the bearer
// token must be a global session; with global auth off they are open, matching
// the rest of the server.
func (h *handler) requireAdmin(w http.ResponseWriter, r *http.Request) bool {
	if !h.o.Auth.Enable {
		return true
	}
	scope, ok := auth.Verify(h.o.Auth.Secret, bearerToken(r))
	if !ok || scope != "session" {
		writeError(w, http.StatusUnauthorized, 401, "unauthorized")
		return false
	}
	return true
}

func (h *handler) addShare(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) {
		return
	}
	body, err := decode[struct {
		Key      string   `json:"key"`
		Name     string   `json:"name"`
		Type     string   `json:"type"`
		AppKeys  []string `json:"appKeys"`
		APIKeys  []string `json:"apiKeys"`
		Password string   `json:"password"`
	}](r)
	if err != nil || body.Name == "" {
		writeError(w, http.StatusBadRequest, 400, "field not found")
		return
	}
	switch body.Type {
	case "all", "app", "api":
	default:
		writeError(w, http.StatusBadRequest, 400, "invalid type")
		return
	}
	if body.Type == "app" && len(body.AppKeys) == 0 {
		writeError(w, http.StatusBadRequest, 400, "appKeys required")
		return
	}
	if body.Type == "api" && len(body.APIKeys) == 0 {
		writeError(w, http.StatusBadRequest, 400, "apiKeys required")
		return
	}

	key := body.Key
	var rec *shareRecord
	if key != "" {
		if !shareKeyRe.MatchString(key) {
			writeError(w, http.StatusBadRequest, 400, "invalid key")
			return
		}
		rec, _ = h.shares.get(key)
	}
	if rec == nil {
		if key == "" {
			if key, err = randomHex(16); err != nil {
				writeError(w, http.StatusInternalServerError, 500, "save failed")
				return
			}
		}
		secret, err := randomHex(32)
		if err != nil {
			writeError(w, http.StatusInternalServerError, 500, "save failed")
			return
		}
		rec = &shareRecord{
			Key: key, Secret: secret,
			CreateAt: time.Now().Format("2006-01-02 15:04:05"),
		}
	}
	rec.Name, rec.Type = body.Name, body.Type
	switch body.Type {
	case "app":
		rec.AppKeys, rec.APIKeys = body.AppKeys, nil
	case "api":
		rec.AppKeys, rec.APIKeys = body.AppKeys, body.APIKeys
	default:
		rec.AppKeys, rec.APIKeys = nil, nil
	}
	// An edit without a password keeps the old one instead of clearing it.
	if body.Password != "" {
		rec.PasswordHash = sharePasswordHash(rec.Secret, body.Password)
	}
	if err := h.shares.put(rec); err != nil {
		writeError(w, http.StatusInternalServerError, 500, "save failed")
		return
	}
	writeJSON(w, http.StatusOK, ok(true))
}

func (h *handler) shareList(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) {
		return
	}
	body, _ := decode[struct {
		PageIndex int `json:"pageIndex"`
	}](r)
	page := body.PageIndex
	if page < 1 {
		page = 1
	}
	records := h.shares.list()
	data := []map[string]any{}
	start := (page - 1) * sharePageSize
	if start < len(records) {
		end := start + sharePageSize
		if end > len(records) {
			end = len(records)
		}
		for _, rec := range records[start:end] {
			data = append(data, shareView(rec))
		}
	}
	writeJSON(w, http.StatusOK, ok(map[string]any{"total": len(records), "data": data}))
}

func (h *handler) shareDetail(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) {
		return
	}
	body, err := decode[struct {
		Key string `json:"key"`
	}](r)
	if err != nil || body.Key == "" {
		writeError(w, http.StatusBadRequest, 400, "field not found")
		return
	}
	rec, found := h.shares.get(body.Key)
	if !found {
		writeError(w, http.StatusNotFound, 404, "share not exists")
		return
	}
	writeJSON(w, http.StatusOK, ok(shareView(rec)))
}

func (h *handler) deleteShare(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) {
		return
	}
	body, err := decode[struct {
		Key string `json:"key"`
	}](r)
	if err != nil || body.Key == "" {
		writeError(w, http.StatusBadRequest, 400, "field not found")
		return
	}
	h.shares.delete(body.Key)
	writeJSON(w, http.StatusOK, ok(true))
}

func (h *handler) shareAction(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) {
		return
	}
	body, err := decode[struct {
		Key   string `json:"key"`
		Index *int   `json:"index"`
	}](r)
	if err != nil || body.Key == "" || body.Index == nil {
		writeError(w, http.StatusBadRequest, 400, "field not found")
		return
	}
	if _, found := h.shares.get(body.Key); !found {
		writeError(w, http.StatusNotFound, 404, "share not exists")
		return
	}
	if *body.Index < 0 || *body.Index >= len(h.o.ShareActions) || h.o.ShareActions[*body.Index].Action == "" {
		writeJSON(w, http.StatusOK, ok(false))
		return
	}
	spec := h.o.ShareActions[*body.Index]
	params := spec.Params
	if params == nil {
		params = map[string]any{}
	}
	writeJSON(w, http.StatusOK, ok(map[string]any{"action": spec.Action, "params": params}))
}

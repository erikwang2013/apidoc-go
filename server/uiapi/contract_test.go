package uiapi

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// Every UIC-1 fixture must be a non-empty list of named success envelopes.
func TestFixturesLoad(t *testing.T) {
	files, err := filepath.Glob("../../contract/uic-1/fixtures/*.json")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("no fixtures")
	}
	for _, file := range files {
		t.Run(filepath.Base(file), func(t *testing.T) {
			b, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			var rows []struct {
				Name     string `json:"name"`
				Response struct {
					Code    *int            `json:"code"`
					Message string          `json:"message"`
					Data    json.RawMessage `json:"data"`
				} `json:"response"`
			}
			if err := json.Unmarshal(b, &rows); err != nil {
				t.Fatal(err)
			}
			if len(rows) == 0 {
				t.Fatal("no fixture rows")
			}
			for _, row := range rows {
				if row.Name == "" {
					t.Errorf("unnamed fixture row")
				}
				if row.Response.Code == nil || *row.Response.Code != 0 {
					t.Errorf("%s: code = %v", row.Name, row.Response.Code)
				}
				if row.Response.Message == "" {
					t.Errorf("%s: empty message", row.Name)
				}
				if len(row.Response.Data) == 0 || string(row.Response.Data) == "null" {
					t.Errorf("%s: missing data", row.Name)
				}
			}
		})
	}
}

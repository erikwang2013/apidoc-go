package uiapi

import (
	"encoding/json"
	"os"
	"testing"
)

func TestFixturesLoad(t *testing.T) {
	b, err := os.ReadFile("../../contract/uic-1/fixtures/menus.json")
	if err != nil {
		t.Fatal(err)
	}
	var rows []struct {
		Name     string `json:"name"`
		Response struct {
			Code int `json:"code"`
		} `json:"response"`
	}
	if err := json.Unmarshal(b, &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) == 0 {
		t.Fatal("no fixtures")
	}
	if rows[0].Response.Code != 0 {
		t.Fatalf("menus fixture code = %d", rows[0].Response.Code)
	}
}

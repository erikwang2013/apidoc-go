package odata

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/erikwang2013/apidoc-go"
)

func TestODataQuery(t *testing.T) {
	var gotMethod, gotFilter, gotSelect, gotTop, gotOrder string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		q := r.URL.Query()
		gotFilter = q.Get("$filter")
		gotSelect = q.Get("$select")
		gotTop = q.Get("$top")
		gotOrder = q.Get("$orderby")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"value":[]}`))
	}))
	defer srv.Close()

	resp, err := New().Exec(context.Background(), apidoc.DebugRequest{
		URL:    srv.URL + "/odata/Products",
		Method: "GET",
		Params: []apidoc.DebugValue{
			{Name: "$filter", In: "query", Value: "Price gt 10"},
			{Name: "$select", In: "query", Value: "Name,Price"},
			{Name: "$top", In: "query", Value: "5"},
		},
		Options: map[string]string{"$orderby": "Name desc"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if gotMethod != http.MethodGet {
		t.Errorf("method = %q", gotMethod)
	}
	if gotFilter != "Price gt 10" || gotSelect != "Name,Price" || gotTop != "5" || gotOrder != "Name desc" {
		t.Errorf("query = filter %q select %q top %q orderby %q", gotFilter, gotSelect, gotTop, gotOrder)
	}
	if resp.Status != "200 OK" || string(resp.Body) != `{"value":[]}` {
		t.Fatalf("response = %+v", resp)
	}
}

func TestODataErrors(t *testing.T) {
	if _, err := New().Exec(context.Background(), apidoc.DebugRequest{
		URL: "http://example.invalid", Method: "POST",
	}); err == nil {
		t.Error("want error for non-GET method")
	}
	if _, err := New().Exec(context.Background(), apidoc.DebugRequest{
		URL: "http://example.invalid", Body: []byte(`{}`),
	}); err == nil {
		t.Error("want error for GET with a body")
	}
	if _, err := New().Exec(context.Background(), apidoc.DebugRequest{
		URL: "ftp://example.invalid",
	}); err == nil {
		t.Error("want error for unsupported scheme")
	}
}

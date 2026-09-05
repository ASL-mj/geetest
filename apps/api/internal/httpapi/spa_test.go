package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func TestDecodeJSONRejectsTrailingValues(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"ok":true}{"extra":true}`))
	var payload map[string]any
	if decodeJSON(rec, req, &payload) {
		t.Fatal("decoder must reject trailing JSON values")
	}
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d", rec.Code)
	}
	var envelope map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("invalid error envelope: %v", err)
	}
}

func TestSPAHandlerServesIndexForHistoryRoutes(t *testing.T) {
	files := fstest.MapFS{
		"index.html":           {Data: []byte("<html>console</html>")},
		"assets/app.js":        {Data: []byte("console.log(1)")},
		"assets/index-abc.css": {Data: []byte("body{}")},
	}
	spa := spaHandlerFor(files)
	if spa == nil {
		t.Fatal("expected SPA handler for non-empty fs")
	}

	for _, route := range []string{"/", "/console/keys", "/admin/users", "/docs"} {
		rec := httptest.NewRecorder()
		spa.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, route, nil))
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "console") {
			t.Fatalf("GET %s should fall back to index.html, got %d %q", route, rec.Code, rec.Body.String())
		}
	}

	rec := httptest.NewRecorder()
	spa.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/assets/app.js", nil))
	if rec.Code != http.StatusOK || rec.Body.String() != "console.log(1)" {
		t.Fatalf("asset should be served directly, got %d %q", rec.Code, rec.Body.String())
	}

	// API namespaces must never receive HTML.
	for _, route := range []string{"/v1/does-not-exist", "/admin/v1/unknown", "/healthz"} {
		rec := httptest.NewRecorder()
		spa.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, route, nil))
		if strings.Contains(rec.Body.String(), "<html>") {
			t.Fatalf("GET %s must not return the SPA index", route)
		}
	}

	if spaHandlerFor(fstest.MapFS{}) != nil {
		t.Fatal("empty web fs should disable SPA serving")
	}
}

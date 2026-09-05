package httpapi

import (
	_ "embed"
	"net/http"
)

// openAPIDocument is the versioned public contract. It deliberately names
// only platform endpoints and never describes the server-only solver gateway.
//
//go:embed openapi-v1.json
var openAPIDocument []byte

func handleOpenAPI(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/vnd.oai.openapi+json;version=3.1")
	w.Header().Set("Cache-Control", "public, max-age=300")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(openAPIDocument)
}

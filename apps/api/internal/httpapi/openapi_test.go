package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOpenAPIContractDeclaresPublicSolveAuthenticationAndIdempotency(t *testing.T) {
	recorder := httptest.NewRecorder()
	handleOpenAPI(recorder, httptest.NewRequest(http.MethodGet, "/openapi.json", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected public OpenAPI document, got %d", recorder.Code)
	}
	var document struct {
		OpenAPI    string                     `json:"openapi"`
		Paths      map[string]json.RawMessage `json:"paths"`
		Components struct {
			Parameters map[string]struct {
				Name     string `json:"name"`
				In       string `json:"in"`
				Required bool   `json:"required"`
			} `json:"parameters"`
		} `json:"components"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &document); err != nil {
		t.Fatalf("OpenAPI response must be valid JSON: %v", err)
	}
	var pathItem struct {
		Post struct {
			Security   []map[string][]string `json:"security"`
			Parameters []struct {
				Reference string `json:"$ref"`
				Name      string `json:"name"`
				In        string `json:"in"`
				Required  bool   `json:"required"`
			} `json:"parameters"`
		} `json:"post"`
	}
	if err := json.Unmarshal(document.Paths["/v1/captcha/solve"], &pathItem); err != nil {
		t.Fatalf("solve path item must be valid JSON: %v", err)
	}
	if document.OpenAPI != "3.1.1" {
		t.Fatalf("unexpected OpenAPI version %q", document.OpenAPI)
	}
	solve := pathItem.Post
	if len(solve.Security) != 1 || solve.Security[0]["bearerAuth"] == nil {
		t.Fatalf("solve operation must require bearer authentication: %#v", solve.Security)
	}
	for _, parameter := range solve.Parameters {
		if parameter.Reference == "#/components/parameters/IdempotencyKey" {
			resolved := document.Components.Parameters["IdempotencyKey"]
			parameter.Name = resolved.Name
			parameter.In = resolved.In
			parameter.Required = resolved.Required
		}
		if parameter.Name == "Idempotency-Key" && parameter.In == "header" && parameter.Required {
			return
		}
	}
	t.Fatal("solve operation must declare a required Idempotency-Key header")
}

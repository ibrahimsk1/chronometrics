package contract

import (
	"bytes"
	"context"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers/gorillamux"
)

func TestContractExamplesValidate(t *testing.T) {
	ctx := context.Background()
	loader := openapi3.NewLoader()
	loader.IsExternalRefsAllowed = true
	doc, err := loader.LoadFromFile(filepath.Join("..", "..", "openapi.yaml"))
	if err != nil {
		t.Fatalf("load openapi: %v", err)
	}
	if err := doc.Validate(ctx); err != nil {
		t.Fatalf("openapi validation failed: %v", err)
	}

	router, err := gorillamux.NewRouter(doc)
	if err != nil {
		t.Fatalf("create router: %v", err)
	}

	cases := []struct {
		examplePath        string
		method             string
		path               string
		validateAsResponse bool
		status             int
	}{
		{"examples/post_events.json", "POST", "/events", false, 202},
		{"examples/post_events_bulk.json", "POST", "/events/bulk", false, 202},
		{"examples/get_metrics.json", "GET", "/metrics", true, 200},
		{"examples/get_health.json", "GET", "/health", true, 200},
	}

	for _, c := range cases {
		c := c
		t.Run(c.examplePath, func(t *testing.T) {
			t.Parallel()
			b, err := os.ReadFile(c.examplePath)
			if err != nil {
				t.Fatalf("read example %s: %v", c.examplePath, err)
			}

			req, err := http.NewRequest(c.method, c.path, bytes.NewReader(b))
			if err != nil {
				t.Fatalf("create request: %v", err)
			}
			req.Header.Set("Content-Type", "application/json")

			route, pathParams, err := router.FindRoute(req)
			if err != nil {
				t.Fatalf("find route for %s %s: %v", c.method, c.path, err)
			}

			reqValidationInput := &openapi3filter.RequestValidationInput{
				Request:    req,
				PathParams: pathParams,
				Route:      route,
			}

			if !c.validateAsResponse {
				if err := openapi3filter.ValidateRequest(ctx, reqValidationInput); err != nil {
					t.Fatalf("request validation failed: %v", err)
				}
				return
			}

			respValidation := &openapi3filter.ResponseValidationInput{
				RequestValidationInput: reqValidationInput,
				Status:                 c.status,
				Header:                 http.Header{"Content-Type": []string{"application/json"}},
			}
			respValidation.SetBodyBytes(b)
			if err := openapi3filter.ValidateResponse(ctx, respValidation); err != nil {
				t.Fatalf("response validation failed: %v", err)
			}
		})
	}
}

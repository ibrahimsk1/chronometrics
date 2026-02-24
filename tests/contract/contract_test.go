package contract

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
)

func TestContractExamplesValidate(t *testing.T) {
	ctx := context.Background()
	loader := &openapi3.Loader{Context: ctx, IsExternalRefsAllowed: true}
	doc, err := loader.LoadFromFile(filepath.Join("..", "..", "openapi.yaml"))
	if err != nil {
		t.Fatalf("load openapi: %v", err)
	}
	if err := doc.Validate(loader.Context); err != nil {
		t.Fatalf("openapi validation failed: %v", err)
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

			// Decode example JSON into a generic value for validation
			var v interface{}
			if err := json.Unmarshal(b, &v); err != nil {
				t.Fatalf("invalid json in %s: %v", c.examplePath, err)
			}

			pathItem := doc.Paths.Value(c.path)
			if pathItem == nil {
				t.Fatalf("path %s not found in spec", c.path)
			}
			var op *openapi3.Operation
			switch c.method {
			case http.MethodGet:
				op = pathItem.Get
			case http.MethodPost:
				op = pathItem.Post
			default:
				t.Fatalf("unsupported method %s", c.method)
			}
			if op == nil {
				t.Fatalf("operation %s %s not declared", c.method, c.path)
			}

			if !c.validateAsResponse {
				if op.RequestBody == nil || op.RequestBody.Value == nil {
					t.Fatalf("no requestBody declared for %s %s", c.method, c.path)
				}
				ct := op.RequestBody.Value.Content.Get("application/json")
				if ct == nil || ct.Schema == nil || ct.Schema.Value == nil {
					t.Fatalf("no application/json schema for requestBody %s %s", c.method, c.path)
				}
				if err := ct.Schema.Value.VisitJSON(v, openapi3.VisitAsRequest()); err != nil {
					t.Fatalf("request example does not match schema: %v", err)
				}
				return
			}

			// Validate as response
			resp := op.Responses.Status(c.status)
			if resp == nil || resp.Value == nil {
				t.Fatalf("no response %d declared for %s %s", c.status, c.method, c.path)
			}
			ct := resp.Value.Content.Get("application/json")
			if ct == nil || ct.Schema == nil || ct.Schema.Value == nil {
				t.Fatalf("no application/json schema for response %d %s %s", c.status, c.method, c.path)
			}
			if err := ct.Schema.Value.VisitJSON(v); err != nil {
				t.Fatalf("response example does not match schema: %v", err)
			}
		})
	}
}

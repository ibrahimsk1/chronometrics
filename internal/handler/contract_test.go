package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestResponse_SchemaMatch_IngestErrors(t *testing.T) {
	ing := &fakeIngesterOK{}
	h := New(ing, nil, nil, ServerConfig{MaxBodyBytes: 1 << 20})
	s := httptest.NewServer(h.Router())
	defer s.Close()

	// invalid JSON -> 400 with ErrorResponse
	res, err := http.Post(s.URL+"/events", "application/json", strings.NewReader("{invalid"))
	if err != nil {
		t.Fatalf("post failed: %v", err)
	}
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", res.StatusCode)
	}
	var er ErrorResponse
	if err := json.NewDecoder(res.Body).Decode(&er); err != nil {
		t.Fatalf("decode error response failed: %v", err)
	}
	if er.Code == "" || er.Message == "" {
		t.Fatalf("error response missing fields: %#v", er)
	}
}

func TestResponse_SchemaMatch_BulkAcceptedShape(t *testing.T) {
	ing := &fakeIngesterOK{}
	h := New(ing, nil, nil, ServerConfig{MaxBodyBytes: 1 << 20})
	s := httptest.NewServer(h.Router())
	defer s.Close()

	payload := `{"events":[{"event_name":"a","user_id":"u1","timestamp":1670000000}]}`
	res, err := http.Post(s.URL+"/events/bulk", "application/json", strings.NewReader(payload))
	if err != nil {
		t.Fatalf("post failed: %v", err)
	}
	if res.StatusCode != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", res.StatusCode)
	}
	var br BulkResponse
	if err := json.NewDecoder(res.Body).Decode(&br); err != nil {
		t.Fatalf("decode bulk response failed: %v", err)
	}
	// expect accepted count >=1 and rejected >=0
	if br.Accepted < 1 {
		t.Fatalf("expected accepted>=1, got %#v", br)
	}
}

func TestResponse_SchemaMatch_Health(t *testing.T) {
	hc := &fakeHealthOK{}
	h := New(nil, nil, hc, ServerConfig{MaxBodyBytes: 1 << 20})
	s := httptest.NewServer(h.Router())
	defer s.Close()

	res, err := http.Get(s.URL + "/health")
	if err != nil {
		t.Fatalf("get failed: %v", err)
	}
	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", res.StatusCode)
	}
	// ensure JSON body contains buffer_strategy
	var m map[string]interface{}
	if err := json.NewDecoder(res.Body).Decode(&m); err != nil {
		t.Fatalf("decode health response failed: %v", err)
	}
	if _, ok := m["buffer_strategy"]; !ok {
		t.Fatalf("health response missing buffer_strategy: %#v", m)
	}
}

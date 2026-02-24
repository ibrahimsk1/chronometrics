package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type fakeHealthOK struct{}

func (f *fakeHealthOK) Health(ctx context.Context) HealthStatus {
	return HealthStatus{
		Status:         "healthy",
		BufferStrategy: "memory",
		ClickHouse:     "connected",
	}
}

func TestHealth_Returns200(t *testing.T) {
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
	// read body to assert presence
	body := make([]byte, 1024)
	n, _ := res.Body.Read(body)
	if !strings.Contains(string(body[:n]), "buffer_strategy") {
		t.Fatalf("expected buffer_strategy in response")
	}
}

func TestMiddleware_BodyTooLarge(t *testing.T) {
	h := New(nil, nil, nil, ServerConfig{MaxBodyBytes: 10})
	s := httptest.NewServer(h.Router())
	defer s.Close()

	payload := strings.Repeat("A", 100)
	res, err := http.Post(s.URL+"/events", "application/json", strings.NewReader(payload))
	if err != nil {
		t.Fatalf("post failed: %v", err)
	}
	if res.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413, got %d", res.StatusCode)
	}
}

package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"eventmetrics/internal/domain"
)

type fakeIngesterOK struct{}

func (f *fakeIngesterOK) Ingest(ctx context.Context, e *domain.RawEvent) error { return nil }

type fakeIngesterFail struct{}

func (f *fakeIngesterFail) Ingest(ctx context.Context, e *domain.RawEvent) error { return domain.ErrPublishFailed }

func TestIngest_ValidEvent(t *testing.T) {
	ing := &fakeIngesterOK{}
	h := New(ing, nil, nil, ServerConfig{MaxBodyBytes: 1 << 20})
	s := httptest.NewServer(h.Router())
	defer s.Close()

	payload := `{"event_name":"purchase","user_id":"u123","timestamp":1670000000}`
	res, err := http.Post(s.URL+"/events", "application/json", strings.NewReader(payload))
	if err != nil {
		t.Fatalf("post failed: %v", err)
	}
	if res.StatusCode != http.StatusAccepted {
		t.Fatalf("expected 202 accepted, got %d", res.StatusCode)
	}
}

func TestIngest_PublishFailed(t *testing.T) {
	ing := &fakeIngesterFail{}
	h := New(ing, nil, nil, ServerConfig{MaxBodyBytes: 1 << 20})
	s := httptest.NewServer(h.Router())
	defer s.Close()

	payload := `{"event_name":"purchase","user_id":"u123","timestamp":1670000000}`
	res, err := http.Post(s.URL+"/events", "application/json", strings.NewReader(payload))
	if err != nil {
		t.Fatalf("post failed: %v", err)
	}
	if res.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", res.StatusCode)
	}
	if res.Header.Get("Retry-After") == "" {
		t.Fatalf("expected Retry-After header on publish failure")
	}
}

func TestIngest_PayloadTooLarge(t *testing.T) {
	ing := &fakeIngesterOK{}
	h := New(ing, nil, nil, ServerConfig{MaxBodyBytes: 10}) // tiny limit
	s := httptest.NewServer(h.Router())
	defer s.Close()

	// payload longer than 10 bytes
	payload := `{"event_name":"purchase","user_id":"u123","timestamp":1670000000}`
	res, err := http.Post(s.URL+"/events", "application/json", strings.NewReader(payload))
	if err != nil {
		t.Fatalf("post failed: %v", err)
	}
	if res.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413, got %d", res.StatusCode)
	}
}

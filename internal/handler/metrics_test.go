package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"eventmetrics/internal/domain"
)

type fakeQuerierOK struct{}

func (f *fakeQuerierOK) Query(ctx context.Context, params domain.QueryParams) (*domain.MetricResult, error) {
	return &domain.MetricResult{
		EventName:   params.EventName,
		From:        params.From,
		To:          params.To,
		GroupBy:     params.GroupBy,
		TotalCount:  1,
		UniqueCount: 1,
	}, nil
}

type fakeQuerierTimeout struct{}

func (f *fakeQuerierTimeout) Query(ctx context.Context, params domain.QueryParams) (*domain.MetricResult, error) {
	return nil, ErrQueryTimeout
}

func TestMetrics_Query_Success(t *testing.T) {
	q := &fakeQuerierOK{}
	h := New(nil, q, nil, ServerConfig{MaxBodyBytes: 1 << 20})
	s := httptest.NewServer(h.Router())
	defer s.Close()

	res, err := http.Get(s.URL + "/metrics?from=0&to=1")
	if err != nil {
		t.Fatalf("get failed: %v", err)
	}
	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", res.StatusCode)
	}
}

func TestMetrics_Query_Timeout(t *testing.T) {
	q := &fakeQuerierTimeout{}
	h := New(nil, q, nil, ServerConfig{MaxBodyBytes: 1 << 20})
	s := httptest.NewServer(h.Router())
	defer s.Close()

	res, err := http.Get(s.URL + "/metrics?from=0&to=1")
	if err != nil {
		t.Fatalf("get failed: %v", err)
	}
	if res.StatusCode != http.StatusGatewayTimeout {
		t.Fatalf("expected 504, got %d", res.StatusCode)
	}
}

func TestMetrics_Query_MissingEventName(t *testing.T) {
	q := &fakeQuerierOK{}
	h := New(nil, q, nil, ServerConfig{MaxBodyBytes: 1 << 20})
	s := httptest.NewServer(h.Router())
	defer s.Close()

	res, err := http.Get(s.URL + "/metrics?from=0&to=1")
	if err != nil {
		t.Fatalf("get failed: %v", err)
	}
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", res.StatusCode)
	}
}

func TestMetrics_Query_MissingFromTo(t *testing.T) {
	q := &fakeQuerierOK{}
	h := New(nil, q, nil, ServerConfig{MaxBodyBytes: 1 << 20})
	s := httptest.NewServer(h.Router())
	defer s.Close()

	res, err := http.Get(s.URL + "/metrics?event_name=evt")
	if err != nil {
		t.Fatalf("get failed: %v", err)
	}
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", res.StatusCode)
	}
}

func TestMetrics_Query_InvalidGroupBy(t *testing.T) {
	q := &fakeQuerierOK{}
	h := New(nil, q, nil, ServerConfig{MaxBodyBytes: 1 << 20})
	s := httptest.NewServer(h.Router())
	defer s.Close()

	res, err := http.Get(s.URL + "/metrics?event_name=evt&from=0&to=1&group_by=bad")
	if err != nil {
		t.Fatalf("get failed: %v", err)
	}
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid group_by, got %d", res.StatusCode)
	}
}

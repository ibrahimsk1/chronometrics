package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

type fakeQuerierOK struct{}

func (f *fakeQuerierOK) Query(ctx context.Context, params map[string][]string) (interface{}, error) {
	return map[string]interface{}{"count": 1}, nil
}

type fakeQuerierTimeout struct{}

func (f *fakeQuerierTimeout) Query(ctx context.Context, params map[string][]string) (interface{}, error) {
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

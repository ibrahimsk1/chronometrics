package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type fakeIngesterSelective struct {
	failIDs map[string]struct{}
}

func (f *fakeIngesterSelective) Ingest(ctx context.Context, e *RawEvent) error {
	if _, ok := f.failIDs[e.UserID]; ok {
		return ErrPublishFailed
	}
	return nil
}

func TestBulk_PartialSuccess(t *testing.T) {
	// fail middle item
	failMap := map[string]struct{}{"u2": {}}
	ing := &fakeIngesterSelective{failIDs: failMap}
	h := New(ing, nil, nil, ServerConfig{MaxBodyBytes: 1 << 20})
	s := httptest.NewServer(h.Router())
	defer s.Close()

	payload := `{"events":[{"event_name":"a","user_id":"u1","timestamp":1670000000},{"event_name":"b","user_id":"u2","timestamp":1670000001},{"event_name":"c","user_id":"u3","timestamp":1670000002}]}`
	res, err := http.Post(s.URL+"/events/bulk", "application/json", strings.NewReader(payload))
	if err != nil {
		t.Fatalf("post failed: %v", err)
	}
	if res.StatusCode != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", res.StatusCode)
	}
}

func TestBulk_AllInvalid(t *testing.T) {
	ing := &fakeIngesterOK{}
	h := New(ing, nil, nil, ServerConfig{MaxBodyBytes: 1 << 20})
	s := httptest.NewServer(h.Router())
	defer s.Close()

	// all items missing required fields
	payload := `{"events":[{"event_name":"","user_id":"","timestamp":0},{"event_name":"","user_id":"","timestamp":0}]}`
	res, err := http.Post(s.URL+"/events/bulk", "application/json", strings.NewReader(payload))
	if err != nil {
		t.Fatalf("post failed: %v", err)
	}
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", res.StatusCode)
	}
}

func TestBulk_PayloadTooLarge(t *testing.T) {
	ing := &fakeIngesterOK{}
	h := New(ing, nil, nil, ServerConfig{MaxBodyBytes: 10}) // tiny limit
	s := httptest.NewServer(h.Router())
	defer s.Close()

	payload := `{"events":[{"event_name":"a","user_id":"u1","timestamp":1670000000}]}`
	res, err := http.Post(s.URL+"/events/bulk", "application/json", strings.NewReader(payload))
	if err != nil {
		t.Fatalf("post failed: %v", err)
	}
	if res.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413, got %d", res.StatusCode)
	}
}

package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"
	"net/url"
	"strings"
)

// Environment & prereqs
// - Start environment once (do NOT tear down per-test):
//   docker compose up -d
//   (default profile runs ingestor + clickhouse; ingestor listens on :8080)
// - Ensure EVENTMETRICS_BUFFER_STRATEGY=memory (default for compose)
// - Tests assume ClickHouse and ingestor are reachable at default ports.
// - Base URL can be overridden with E2E_BASE_URL env var (defaults to http://localhost:8080).
//
// Test strategy
// - Tests are pure Go `go test` files. They assume containers remain running between tests.
// - Tests use unique event_name per run to avoid the need to truncate ClickHouse.
// - Poll /metrics up to 5s for eventual persistence visibility (flush interval ~100ms).

var baseURL = func() string {
	if v := os.Getenv("E2E_BASE_URL"); v != "" {
		return v
	}
	return "http://localhost:8080"
}()

var clickhouseURL = func() string {
	if v := os.Getenv("E2E_CLICKHOUSE_URL"); v != "" {
		return v
	}
	return "http://localhost:8123"
}()

var clickhouseDB = func() string {
	if v := os.Getenv("E2E_CLICKHOUSE_DB"); v != "" {
		return v
	}
	return "default"
}()

type EventPayload struct {
	EventName  string                 `json:"event_name"`
	UserID     string                 `json:"user_id"`
	Timestamp  int64                  `json:"timestamp"`
	Channel    string                 `json:"channel,omitempty"`
	CampaignID string                 `json:"campaign_id,omitempty"`
	Tags       []string               `json:"tags,omitempty"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
}

type MetricsResponse struct {
	EventName   string `json:"event_name"`
	From        uint64 `json:"from"`
	To          uint64 `json:"to"`
	GroupBy     string `json:"group_by,omitempty"`
	TotalCount  uint64 `json:"total_count"`
	UniqueCount uint64 `json:"unique_count"`
}

func postEvent(t *testing.T, evt EventPayload) {
	t.Helper()
	b, err := json.Marshal(evt)
	if err != nil {
		t.Fatalf("marshal event: %v", err)
	}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, baseURL+"/events", bytes.NewReader(b))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post /events: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("expected 202 Accepted, got %d", resp.StatusCode)
	}
}

func getMetrics(t *testing.T, eventName string, from, to uint64) (MetricsResponse, int) {
	t.Helper()
	url := fmt.Sprintf("%s/metrics?event_name=%s&from=%d&to=%d", baseURL, eventName, from, to)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("get /metrics: %v", err)
	}
	defer resp.Body.Close()
	var mr MetricsResponse
	if resp.StatusCode == http.StatusOK {
		if err := json.NewDecoder(resp.Body).Decode(&mr); err != nil {
			t.Fatalf("decode metrics response: %v", err)
		}
	}
	return mr, resp.StatusCode
}

// TestE2E_HappyPath_SingleEventIngestion verifies that a valid single event is accepted and persisted.
func TestE2E_HappyPath_SingleEventIngestion(t *testing.T) {
	now := time.Now()
	eventName := fmt.Sprintf("e2e_test_event_%d", now.UnixNano())
	userID := "user_e2e_1"

	evt := EventPayload{
		EventName: eventName,
		UserID:    userID,
		Timestamp: now.UnixMilli(),
		Channel:   "web",
		Tags:      []string{"e2e", "happypath"},
		Metadata: map[string]interface{}{
			"note": "e2e single event ingestion",
		},
	}

	// Ensure we attempt cleanup of test rows after test completes.
	cleanupNeeded := false
	defer func() {
		if cleanupNeeded {
			if err := cleanupEvent(eventName); err != nil {
				t.Logf("cleanup failed: %v", err)
			}
		}
	}()

	// POST event
	postEvent(t, evt)

	// Poll /metrics until total_count == 1 or timeout
	from := uint64(now.Add(-5 * time.Second).UnixMilli())
	to := uint64(now.Add(10 * time.Second).UnixMilli())
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		mr, status := getMetrics(t, eventName, from, to)
		if status == http.StatusOK {
			if mr.TotalCount == 1 && mr.UniqueCount == 1 {
				// success — mark for cleanup and exit
				cleanupNeeded = true
				return
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("event %q not persisted as expected within timeout", eventName)
}

// cleanupEvent deletes rows for the given event_name from ClickHouse using the HTTP interface.
func cleanupEvent(eventName string) error {
	query := fmt.Sprintf("ALTER TABLE %s.events DELETE WHERE event_name = '%s'", clickhouseDB, strings.ReplaceAll(eventName, "'", "\\'"))
	// ClickHouse accepts SQL in the POST body.
	u := clickhouseURL
	// If user provided just host without path, use root.
	if _, err := url.ParseRequestURI(u); err != nil {
		return fmt.Errorf("invalid clickhouse url %q: %w", u, err)
	}
	resp, err := http.Post(u, "text/plain", strings.NewReader(query))
	if err != nil {
		return fmt.Errorf("post clickhouse query: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("clickhouse returned status %d for cleanup query", resp.StatusCode)
	}
	return nil
}


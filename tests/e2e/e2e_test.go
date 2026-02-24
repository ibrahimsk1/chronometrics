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
	"io"
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
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("expected 202 Accepted, got %d", resp.StatusCode)
	}
}

// postRaw posts arbitrary JSON and returns status and body.
func postRaw(t *testing.T, payload []byte) (int, string) {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, baseURL+"/events", bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post /events: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
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
	defer func() { _ = resp.Body.Close() }()
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
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("clickhouse returned status %d for cleanup query", resp.StatusCode)
	}
	return nil
}

// TestE2E_ValidationFailures verifies 400 responses for invalid payloads.
func TestE2E_ValidationFailures(t *testing.T) {
	// missing event_name
	payload := []byte(`{"user_id":"user_x","timestamp":` + fmt.Sprintf("%d", time.Now().UnixMilli()) + `}`)
	status, body := postRaw(t, payload)
	if status != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing event_name, got %d body=%s", status, body)
	}

	// invalid timestamp (<=0)
	payload2 := []byte(`{"event_name":"e2e_bad_ts","user_id":"user_x","timestamp":0}`)
	status2, body2 := postRaw(t, payload2)
	if status2 != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid timestamp, got %d body=%s", status2, body2)
	}
}

// TestE2E_Deduplication verifies posting the same event twice results in a single persisted row.
func TestE2E_Deduplication(t *testing.T) {
	now := time.Now()
	eventName := fmt.Sprintf("e2e_dedup_event_%d", now.UnixNano())
	userID := "user_e2e_dup"

	evt := EventPayload{
		EventName: eventName,
		UserID:    userID,
		Timestamp: now.UnixMilli(),
		Channel:   "web",
		Tags:      []string{"e2e", "dedup"},
	}

	cleanupNeeded := false
	defer func() {
		if cleanupNeeded {
			if err := cleanupEvent(eventName); err != nil {
				t.Logf("cleanup failed: %v", err)
			}
		}
	}()

	// post twice
	postEvent(t, evt)
	postEvent(t, evt)

	// Poll /metrics until total_count == 1 or timeout
	from := uint64(now.Add(-5 * time.Second).UnixMilli())
	to := uint64(now.Add(10 * time.Second).UnixMilli())
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		mr, status := getMetrics(t, eventName, from, to)
		if status == http.StatusOK {
			if mr.TotalCount == 1 && mr.UniqueCount == 1 {
				cleanupNeeded = true
				return
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("dedup failed: event %q persisted more than once or not at all within timeout", eventName)
}

type GroupResult struct {
	Key         string `json:"key"`
	TotalCount  uint64 `json:"total_count"`
	UniqueCount uint64 `json:"unique_count"`
}

type ExtendedMetricsResponse struct {
	MetricsResponse
	Groups []GroupResult `json:"groups,omitempty"`
}

// postBulk posts a bulk events payload to /events/bulk.
func postBulk(t *testing.T, payload []byte) (int, string) {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, baseURL+"/events/bulk", bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("new bulk request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post /events/bulk: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

// TestE2E_BulkIngestion posts mixed valid/invalid events and verifies accepted persist.
func TestE2E_BulkIngestion(t *testing.T) {
	now := time.Now()
	eventName := fmt.Sprintf("e2e_bulk_event_%d", now.UnixNano())
	valid := EventPayload{
		EventName: eventName,
		UserID:    "bulk_user_1",
		Timestamp: now.UnixMilli(),
		Channel:   "web",
	}
	invalid := map[string]interface{}{
		"event_name": "",
		"user_id":    "x",
		"timestamp":  0,
	}
	bulk := map[string]interface{}{
		"events": []interface{}{
			valid,
			func() interface{} { v := valid; v.UserID = "bulk_user_2"; return v }(),
			invalid,
			func() interface{} { v := valid; v.UserID = "bulk_user_3"; return v }(),
		},
	}
	b, err := json.Marshal(bulk)
	if err != nil {
		t.Fatalf("marshal bulk: %v", err)
	}
	status, body := postBulk(t, b)
	if status != http.StatusAccepted {
		t.Fatalf("expected 202 Accepted for bulk, got %d body=%s", status, body)
	}

	// Poll metrics for accepted count = 3
	from := uint64(now.Add(-5 * time.Second).UnixMilli())
	to := uint64(now.Add(10 * time.Second).UnixMilli())
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		mr, status := getMetrics(t, eventName, from, to)
		if status == http.StatusOK && mr.TotalCount == 3 {
			_ = cleanupEvent(eventName)
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("bulk ingestion events not persisted as expected within timeout")
}

// TestE2E_MetricsGrouping verifies group_by=channel returns correct groups.
func TestE2E_MetricsGrouping(t *testing.T) {
	now := time.Now()
	eventName := fmt.Sprintf("e2e_group_event_%d", now.UnixNano())
	events := []EventPayload{
		{EventName: eventName, UserID: "u1", Timestamp: now.UnixMilli(), Channel: "web"},
		{EventName: eventName, UserID: "u2", Timestamp: now.UnixMilli(), Channel: "web"},
		{EventName: eventName, UserID: "u3", Timestamp: now.UnixMilli(), Channel: "mobile"},
	}
	for _, e := range events {
		postEvent(t, e)
	}

	from := uint64(now.Add(-5 * time.Second).UnixMilli())
	to := uint64(now.Add(10 * time.Second).UnixMilli())
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		url := fmt.Sprintf("%s/metrics?event_name=%s&from=%d&to=%d&group_by=channel", baseURL, eventName, from, to)
		req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("get /metrics group_by: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			_ = resp.Body.Close()
			time.Sleep(200 * time.Millisecond)
			continue
		}
		var emr ExtendedMetricsResponse
		if err := json.NewDecoder(resp.Body).Decode(&emr); err != nil {
			_ = resp.Body.Close()
			t.Fatalf("decode grouped metrics: %v", err)
		}
		_ = resp.Body.Close()
		if emr.TotalCount == 3 && len(emr.Groups) >= 2 {
			var webCount, mobileCount uint64
			for _, g := range emr.Groups {
				if g.Key == "web" {
					webCount = g.TotalCount
				}
				if g.Key == "mobile" {
					mobileCount = g.TotalCount
				}
			}
			if webCount == 2 && mobileCount == 1 {
				_ = cleanupEvent(eventName)
				return
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("grouped metrics did not reflect expected grouping within timeout")
}

// TestE2E_HealthEndpoint verifies /health reflects ClickHouse connectivity.
func TestE2E_HealthEndpoint(t *testing.T) {
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, baseURL+"/health", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("health request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from /health, got %d", resp.StatusCode)
	}
	var body map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode health: %v", err)
	}
	if body["clickhouse"] == "disconnected" {
		t.Fatalf("clickhouse reported disconnected in health: %v", body)
	}
}

// Disruptive tests (backpressure, graceful shutdown) are skipped unless enabled explicitly.
func TestE2E_Backpressure(t *testing.T) {
	if os.Getenv("ENABLE_DISRUPTIVE_TESTS") != "1" {
		t.Skip("skipping disruptive backpressure test; set ENABLE_DISRUPTIVE_TESTS=1 to enable")
	}
	// left as manual/disruptive test
}

func TestE2E_GracefulShutdown(t *testing.T) {
	if os.Getenv("ENABLE_DISRUPTIVE_TESTS") != "1" {
		t.Skip("skipping disruptive graceful-shutdown test; set ENABLE_DISRUPTIVE_TESTS=1 to enable")
	}
	// left as manual/disruptive test
}


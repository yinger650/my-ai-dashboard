// Package sender delivers spooled events to board-server.
package sender

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"agentboard/internal/client/spool"
)

// Sender posts batches to the ingest API.
type Sender struct {
	BaseURL string
	Token   string
	Client  *http.Client
}

// New creates a Sender.
func New(baseURL, token string, timeout time.Duration) *Sender {
	return &Sender{
		BaseURL: strings.TrimRight(baseURL, "/"),
		Token:   token,
		Client:  &http.Client{Timeout: timeout},
	}
}

type ingestResult struct {
	EventID string `json:"event_id"`
	Status  string `json:"status"`
	Code    string `json:"code"`
}

type ingestResponse struct {
	Data struct {
		Accepted   int            `json:"accepted"`
		Duplicates int            `json:"duplicates"`
		Rejected   int            `json:"rejected"`
		Results    []ingestResult `json:"results"`
	} `json:"data"`
}

// Ping verifies connectivity and auth. Returns (retriable, err).
func (s *Sender) Ping(ctx context.Context) (bool, error) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, s.BaseURL+"/ingest/v1/ping", nil)
	req.Header.Set("Authorization", "Bearer "+s.Token)
	resp, err := s.Client.Do(req)
	if err != nil {
		return true, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return false, fmt.Errorf("auth failed: %d", resp.StatusCode)
	}
	if resp.StatusCode >= 400 {
		return true, fmt.Errorf("ping status %d", resp.StatusCode)
	}
	return false, nil
}

// SendResult reports the outcome of a batch send.
type SendResult struct {
	DeleteIDs []string // accepted, duplicate, or permanently rejected
	RetryIDs  []string // transient failures to retry with backoff
	Err       error
}

// Send posts a batch and classifies each event by result.
func (s *Sender) Send(ctx context.Context, batch []spool.QueuedEvent) SendResult {
	if len(batch) == 0 {
		return SendResult{}
	}
	raws := make([]json.RawMessage, 0, len(batch))
	ids := make([]string, 0, len(batch))
	for _, e := range batch {
		raws = append(raws, json.RawMessage(e.Payload))
		ids = append(ids, e.EventID)
	}
	body, _ := json.Marshal(map[string]any{"events": raws})

	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, s.BaseURL+"/ingest/v1/events", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+s.Token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.Client.Do(req)
	if err != nil {
		return SendResult{RetryIDs: ids, Err: err}
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)

	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
		return SendResult{RetryIDs: ids, Err: fmt.Errorf("server status %d", resp.StatusCode)}
	}
	if resp.StatusCode >= 400 {
		// 4xx (auth, bad request): do not hammer; retry after backoff.
		return SendResult{RetryIDs: ids, Err: fmt.Errorf("client status %d: %s", resp.StatusCode, truncate(string(data), 200))}
	}

	var ir ingestResponse
	if err := json.Unmarshal(data, &ir); err != nil {
		return SendResult{RetryIDs: ids, Err: err}
	}
	var del []string
	for _, r := range ir.Data.Results {
		switch r.Status {
		case "accepted", "duplicate", "rejected":
			del = append(del, r.EventID)
		}
	}
	return SendResult{DeleteIDs: del}
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

// Backoff returns the delay for a given attempt using the spec schedule.
func Backoff(attempt int) time.Duration {
	steps := []time.Duration{5 * time.Second, 15 * time.Second, 30 * time.Second, time.Minute, 2 * time.Minute, 5 * time.Minute}
	if attempt < len(steps) {
		return steps[attempt]
	}
	return 15 * time.Minute
}

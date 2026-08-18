package store

import (
	"strings"
	"time"

	"agentboard/internal/shared"
)

// ApplyTTL overlays a stale/warning state when last_seen_at is older than ttl_seconds.
// It mutates the in-memory Service only; the database row is left unchanged so a
// later heartbeat can restore the live state.
func (s *Service) ApplyTTL(now time.Time) {
	if s == nil || s.TTLSeconds == nil || *s.TTLSeconds <= 0 || s.LastSeenAt == nil {
		return
	}
	switch s.CurrentState {
	case "failed", "stopped", "disabled":
		return
	}
	last, err := shared.ParseTime(*s.LastSeenAt)
	if err != nil {
		return
	}
	if now.Sub(last) <= time.Duration(*s.TTLSeconds)*time.Second {
		return
	}
	s.CurrentState = "stale"
	s.Severity = "warning"
	if s.StateSummary == "" {
		s.StateSummary = "TTL 过期，可能已停止上报"
		return
	}
	if !strings.Contains(s.StateSummary, "TTL 过期") {
		s.StateSummary = s.StateSummary + " · TTL 过期"
	}
}

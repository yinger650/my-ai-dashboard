package server

import (
	"sync"
	"time"
)

// tokenBucket is a simple refilling token bucket.
type tokenBucket struct {
	tokens     float64
	capacity   float64
	refillRate float64 // tokens per second
	last       time.Time
}

func (b *tokenBucket) allow(now time.Time) bool {
	elapsed := now.Sub(b.last).Seconds()
	b.last = now
	b.tokens += elapsed * b.refillRate
	if b.tokens > b.capacity {
		b.tokens = b.capacity
	}
	if b.tokens >= 1 {
		b.tokens--
		return true
	}
	return false
}

// rateLimiter holds per-key token buckets with periodic pruning.
type rateLimiter struct {
	mu      sync.Mutex
	buckets map[string]*tokenBucket
}

func newRateLimiter() *rateLimiter {
	return &rateLimiter{buckets: make(map[string]*tokenBucket)}
}

// Allow reports whether a request for key is allowed given a per-minute limit.
func (r *rateLimiter) Allow(key string, perMinute int) bool {
	if perMinute <= 0 {
		return true
	}
	now := time.Now()
	r.mu.Lock()
	defer r.mu.Unlock()
	b, ok := r.buckets[key]
	if !ok {
		b = &tokenBucket{tokens: float64(perMinute), capacity: float64(perMinute), refillRate: float64(perMinute) / 60.0, last: now}
		r.buckets[key] = b
	}
	return b.allow(now)
}

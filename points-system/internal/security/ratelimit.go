package security

import (
	"sync"
	"time"
)

type bucket struct {
	windowStart time.Time
	count       int
}

type RateLimiter struct {
	mu      sync.Mutex
	buckets map[string]bucket
	now     func() time.Time
}

func NewRateLimiter() *RateLimiter {
	return &RateLimiter{buckets: make(map[string]bucket), now: time.Now}
}

func (l *RateLimiter) Allow(key string, limit int, window time.Duration) bool {
	if l == nil || limit <= 0 || window <= 0 {
		return false
	}
	now := l.now()
	l.mu.Lock()
	defer l.mu.Unlock()
	current := l.buckets[key]
	if current.windowStart.IsZero() || now.Sub(current.windowStart) >= window {
		current = bucket{windowStart: now}
	}
	if current.count >= limit {
		l.buckets[key] = current
		return false
	}
	current.count++
	l.buckets[key] = current
	if len(l.buckets) > 10000 {
		for candidate, value := range l.buckets {
			if now.Sub(value.windowStart) > 2*window {
				delete(l.buckets, candidate)
			}
		}
	}
	return true
}

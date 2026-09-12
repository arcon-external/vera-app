package auth

import (
	"sync"
	"time"
)

type attempt struct {
	failures int
	blocked  time.Time
}

// RateLimiter blocks a key after maxFailures consecutive failures until the
// cooldown passes. A success resets the counter.
type RateLimiter struct {
	mu          sync.Mutex
	maxFailures int
	cooldown    time.Duration
	attempts    map[string]*attempt
}

func NewRateLimiter(maxFailures int, cooldown time.Duration) *RateLimiter {
	return &RateLimiter{
		maxFailures: maxFailures,
		cooldown:    cooldown,
		attempts:    make(map[string]*attempt),
	}
}

func (l *RateLimiter) Blocked(key string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	a, ok := l.attempts[key]
	if !ok {
		return false
	}
	return a.failures >= l.maxFailures && now.Before(a.blocked)
}

func (l *RateLimiter) RecordFailure(key string, now time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	a, ok := l.attempts[key]
	if !ok {
		a = &attempt{}
		l.attempts[key] = a
	}
	a.failures++
	if a.failures >= l.maxFailures {
		a.blocked = now.Add(l.cooldown)
	}
}

func (l *RateLimiter) RecordSuccess(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.attempts, key)
}

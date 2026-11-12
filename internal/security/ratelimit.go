package security

import (
	"sync"
	"time"
)

type Limiter struct {
	mu        sync.Mutex
	limit     int
	window    time.Duration
	entries   map[string]*limiterEntry
	lastSweep time.Time
}

type limiterEntry struct {
	count   int
	resetAt time.Time
}

func NewLimiter(limit int, window time.Duration) *Limiter {
	return &Limiter{
		limit:     limit,
		window:    window,
		entries:   make(map[string]*limiterEntry),
		lastSweep: time.Now(),
	}
}

func (l *Limiter) Allow(key string) (bool, time.Duration) {
	now := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()

	l.sweepLocked(now)
	e, ok := l.entries[key]
	if !ok || now.After(e.resetAt) {
		l.entries[key] = &limiterEntry{count: 1, resetAt: now.Add(l.window)}
		return true, 0
	}
	if e.count >= l.limit {
		return false, e.resetAt.Sub(now)
	}
	e.count++
	return true, 0
}

func (l *Limiter) Fail(key string) {
	now := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()
	e, ok := l.entries[key]
	if !ok || now.After(e.resetAt) {
		l.entries[key] = &limiterEntry{count: 1, resetAt: now.Add(l.window)}
		return
	}
	e.count++
}

func (l *Limiter) Reset(key string) {
	l.mu.Lock()
	delete(l.entries, key)
	l.mu.Unlock()
}

func (l *Limiter) sweepLocked(now time.Time) {
	if now.Sub(l.lastSweep) < l.window {
		return
	}
	l.lastSweep = now
	for k, e := range l.entries {
		if now.After(e.resetAt) {
			delete(l.entries, k)
		}
	}
}

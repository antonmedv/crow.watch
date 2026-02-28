package ratelimit

import (
	"sync"
	"time"
)

type entry struct {
	timestamps []time.Time
}

// Limiter implements an in-memory sliding window rate limiter.
type Limiter struct {
	mu      sync.Mutex
	entries map[string]*entry
	max     int
	window  time.Duration
}

// New creates a Limiter that allows max attempts per window.
func New(max int, window time.Duration) *Limiter {
	return &Limiter{
		entries: make(map[string]*entry),
		max:     max,
		window:  window,
	}
}

// Allow checks whether a request for the given key is allowed.
// If allowed, it records the attempt and returns true.
// Denied attempts are not recorded, keeping lists bounded at max entries.
func (l *Limiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-l.window)

	e, ok := l.entries[key]
	if !ok {
		l.entries[key] = &entry{timestamps: []time.Time{now}}
		return true
	}

	// Prune expired timestamps.
	valid := e.timestamps[:0]
	for _, t := range e.timestamps {
		if t.After(cutoff) {
			valid = append(valid, t)
		}
	}
	e.timestamps = valid

	if len(e.timestamps) >= l.max {
		return false
	}

	e.timestamps = append(e.timestamps, now)
	return true
}

// Reset clears all recorded attempts for a key (e.g. on successful login).
func (l *Limiter) Reset(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.entries, key)
}

// Cleanup removes entries whose timestamps have all expired.
func (l *Limiter) Cleanup() {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-l.window)

	for key, e := range l.entries {
		hasValid := false
		for _, t := range e.timestamps {
			if t.After(cutoff) {
				hasValid = true
				break
			}
		}
		if !hasValid {
			delete(l.entries, key)
		}
	}
}

// StartCleanup runs Cleanup periodically until done is closed.
func (l *Limiter) StartCleanup(interval time.Duration, done <-chan struct{}) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				l.Cleanup()
			case <-done:
				return
			}
		}
	}()
}

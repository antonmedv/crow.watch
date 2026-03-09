package app

import (
	"sync"
	"time"
)

// VisitorCounter tracks unique IPs seen in the last 24 hours.
type VisitorCounter struct {
	mu   sync.Mutex
	seen map[string]time.Time // IP → last seen
}

func NewVisitorCounter() *VisitorCounter {
	return &VisitorCounter{
		seen: make(map[string]time.Time),
	}
}

// Record adds an IP with the current timestamp.
func (vc *VisitorCounter) Record(ip string) {
	now := time.Now()

	vc.mu.Lock()
	defer vc.mu.Unlock()

	vc.seen[ip] = now
}

// Count returns the number of unique IPs seen in the last 24 hours.
func (vc *VisitorCounter) Count() int {
	cutoff := time.Now().Add(-24 * time.Hour)

	vc.mu.Lock()
	defer vc.mu.Unlock()

	count := 0
	for ip, t := range vc.seen {
		if t.After(cutoff) {
			count++
		} else {
			delete(vc.seen, ip)
		}
	}
	return count
}

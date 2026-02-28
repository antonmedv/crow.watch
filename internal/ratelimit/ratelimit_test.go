package ratelimit

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAllow_WithinLimit(t *testing.T) {
	l := New(3, time.Minute)
	assert.True(t, l.Allow("k"))
	assert.True(t, l.Allow("k"))
	assert.True(t, l.Allow("k"))
}

func TestAllow_ExceedsLimit(t *testing.T) {
	l := New(2, time.Minute)
	assert.True(t, l.Allow("k"))
	assert.True(t, l.Allow("k"))
	assert.False(t, l.Allow("k"))
	assert.False(t, l.Allow("k")) // still denied
}

func TestAllow_SeparateKeys(t *testing.T) {
	l := New(1, time.Minute)
	assert.True(t, l.Allow("a"))
	assert.False(t, l.Allow("a"))
	assert.True(t, l.Allow("b")) // different key is independent
}

func TestReset(t *testing.T) {
	l := New(1, time.Minute)
	assert.True(t, l.Allow("k"))
	assert.False(t, l.Allow("k"))

	l.Reset("k")
	assert.True(t, l.Allow("k")) // allowed again after reset
}

func TestCleanup(t *testing.T) {
	l := New(5, 10*time.Millisecond)
	l.Allow("a")
	l.Allow("b")

	time.Sleep(20 * time.Millisecond)
	l.Cleanup()

	l.mu.Lock()
	count := len(l.entries)
	l.mu.Unlock()

	assert.Equal(t, 0, count, "stale entries should be removed")
}

func TestWindowExpiry(t *testing.T) {
	l := New(1, 20*time.Millisecond)
	assert.True(t, l.Allow("k"))
	assert.False(t, l.Allow("k"))

	time.Sleep(30 * time.Millisecond)
	assert.True(t, l.Allow("k"), "should allow after window expires")
}

func TestDeniedAttemptsNotRecorded(t *testing.T) {
	l := New(2, time.Minute)
	require.True(t, l.Allow("k"))
	require.True(t, l.Allow("k"))

	// Denied attempts should not grow the list.
	for i := 0; i < 10; i++ {
		l.Allow("k")
	}

	l.mu.Lock()
	n := len(l.entries["k"].timestamps)
	l.mu.Unlock()

	assert.Equal(t, 2, n, "denied attempts must not be recorded")
}

func TestStartCleanup(t *testing.T) {
	l := New(5, 10*time.Millisecond)
	l.Allow("x")

	done := make(chan struct{})
	l.StartCleanup(15*time.Millisecond, done)

	time.Sleep(40 * time.Millisecond)
	close(done)

	l.mu.Lock()
	count := len(l.entries)
	l.mu.Unlock()

	assert.Equal(t, 0, count, "background cleanup should remove stale entries")
}

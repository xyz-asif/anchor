package ratelimit

import (
	"sync"
	"time"
)

// RateLimiter represents a rate limiter instance
type RateLimiter struct {
	requests map[string][]time.Time
	limit    int
	window   time.Duration
	mu       sync.RWMutex
}

// New creates a new rate limiter
func New(limit int, window time.Duration) *RateLimiter {
	return &RateLimiter{
		requests: make(map[string][]time.Time),
		limit:    limit,
		window:   window,
	}
}

// Allow checks if a request is allowed for the given key
func (rl *RateLimiter) Allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-rl.window)

	// Clean old requests
	var valid []time.Time
	for _, t := range rl.requests[key] {
		if t.After(cutoff) {
			valid = append(valid, t)
		}
	}

	if len(valid) >= rl.limit {
		return false
	}

	rl.requests[key] = append(valid, now)
	return true
}

// GetRemaining returns the number of remaining requests for the given key
func (rl *RateLimiter) GetRemaining(key string) int {
	rl.mu.RLock()
	defer rl.mu.RUnlock()

	now := time.Now()
	cutoff := now.Add(-rl.window)

	var valid int
	for _, t := range rl.requests[key] {
		if t.After(cutoff) {
			valid++
		}
	}

	remaining := rl.limit - valid
	if remaining < 0 {
		remaining = 0
	}

	return remaining
}

// GetResetTime returns the time when the rate limit will reset for the given key
func (rl *RateLimiter) GetResetTime(key string) time.Time {
	rl.mu.RLock()
	defer rl.mu.RUnlock()

	now := time.Now()
	cutoff := now.Add(-rl.window)

	var oldest time.Time
	for _, t := range rl.requests[key] {
		if t.After(cutoff) {
			if oldest.IsZero() || t.Before(oldest) {
				oldest = t
			}
		}
	}

	if oldest.IsZero() {
		return now
	}

	return oldest.Add(rl.window)
}

// Reset clears the rate limit for the given key
func (rl *RateLimiter) Reset(key string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	delete(rl.requests, key)
}

// ResetAll clears all rate limits
func (rl *RateLimiter) ResetAll() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	rl.requests = make(map[string][]time.Time)
}

// GetStats returns statistics about the rate limiter
func (rl *RateLimiter) GetStats() map[string]interface{} {
	rl.mu.RLock()
	defer rl.mu.RUnlock()

	now := time.Now()
	cutoff := now.Add(-rl.window)

	stats := map[string]interface{}{
		"limit":  rl.limit,
		"window": rl.window.String(),
		"keys":   len(rl.requests),
	}

	var totalRequests int
	var activeKeys int

	for _, requests := range rl.requests {
		var valid int
		for _, t := range requests {
			if t.After(cutoff) {
				valid++
			}
		}

		if valid > 0 {
			activeKeys++
			totalRequests += valid
		}
	}

	stats["activeKeys"] = activeKeys
	stats["totalRequests"] = totalRequests

	return stats
}

// Cleanup removes expired entries to prevent memory leaks
func (rl *RateLimiter) Cleanup() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-rl.window)

	for key, requests := range rl.requests {
		var valid []time.Time
		for _, t := range requests {
			if t.After(cutoff) {
				valid = append(valid, t)
			}
		}

		if len(valid) == 0 {
			delete(rl.requests, key)
		} else {
			rl.requests[key] = valid
		}
	}
}

// StartCleanup starts a background cleanup routine
func (rl *RateLimiter) StartCleanup(interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for range ticker.C {
			rl.Cleanup()
		}
	}()
}

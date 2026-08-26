package main

import (
	"sync"
	"time"
)

type requestWindow struct {
	Count     int
	StartedAt time.Time
	LastSeen  time.Time
}

type requestLimiter struct {
	mu      sync.Mutex
	windows map[string]requestWindow
}

func newRequestLimiter() *requestLimiter {
	return &requestLimiter{windows: map[string]requestWindow{}}
}

func (l *requestLimiter) allow(key string, limit int, window time.Duration) bool {
	now := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()
	item := l.windows[key]
	if item.StartedAt.IsZero() || now.Sub(item.StartedAt) >= window {
		item = requestWindow{StartedAt: now}
	}
	item.LastSeen = now
	if item.Count >= limit {
		l.windows[key] = item
		l.prune(now, window)
		return false
	}
	item.Count++
	l.windows[key] = item
	l.prune(now, window)
	return true
}

func (l *requestLimiter) prune(now time.Time, longestWindow time.Duration) {
	if len(l.windows) <= 5000 {
		return
	}
	for key, item := range l.windows {
		if now.Sub(item.LastSeen) > longestWindow {
			delete(l.windows, key)
		}
	}
}

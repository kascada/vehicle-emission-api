package cache

import (
	"context"
	"sync"
	"time"
)

// RateLimiter begrenzt Anfragen pro Schlüssel (z.B. E-Mail) innerhalb eines Zeitfensters.
type RateLimiter struct {
	mu      sync.Mutex
	entries map[string]*rateWindow
	limit   int
	window  time.Duration
}

type rateWindow struct {
	count  int
	start  time.Time
}

// NewRateLimiter erstellt einen neuen RateLimiter.
// limit ist die maximale Anzahl Anfragen pro Zeitfenster.
func NewRateLimiter(limit int, window time.Duration, ctx context.Context) *RateLimiter {
	rl := &RateLimiter{
		entries: make(map[string]*rateWindow),
		limit:   limit,
		window:  window,
	}
	go rl.runCleanup(ctx)
	return rl
}

// Allow prüft ob ein Request für den gegebenen Schlüssel erlaubt ist.
// Gibt true zurück wenn erlaubt, false bei Überschreitung.
func (rl *RateLimiter) Allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	w, exists := rl.entries[key]

	if !exists || now.Sub(w.start) >= rl.window {
		rl.entries[key] = &rateWindow{count: 1, start: now}
		return true
	}

	w.count++
	return w.count <= rl.limit
}

// runCleanup entfernt abgelaufene Einträge alle 5 Minuten.
func (rl *RateLimiter) runCleanup(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			rl.mu.Lock()
			now := time.Now()
			for key, w := range rl.entries {
				if now.Sub(w.start) >= rl.window {
					delete(rl.entries, key)
				}
			}
			rl.mu.Unlock()
		}
	}
}

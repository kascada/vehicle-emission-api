package cache

import (
	"context"
	"sync"
	"time"
)

// VehicleCache speichert serialisierte Fahrzeug-API-Responses (JSON-Bytes).
// Eviction: FIFO bei maxSize. Reset: alle 30 Tage komplett.
type VehicleCache struct {
	mu      sync.RWMutex
	entries map[string][]byte
	order   []string
	maxSize int
}

// NewVehicleCache erstellt den Cache und startet die Reset-Goroutine.
// Die Goroutine stoppt sauber bei Context-Cancel.
func NewVehicleCache(maxSize int, ctx context.Context) *VehicleCache {
	c := &VehicleCache{
		entries: make(map[string][]byte),
		maxSize: maxSize,
	}
	go c.runReset(ctx)
	return c
}

// Get liefert gecachte Response-Bytes für eine Fahrzeug-ID.
func (c *VehicleCache) Get(id string) ([]byte, bool) {
	c.mu.RLock()
	data, ok := c.entries[id]
	c.mu.RUnlock()
	return data, ok
}

// Set speichert Response-Bytes. Bei maxSize wird der älteste Eintrag entfernt (FIFO).
func (c *VehicleCache) Set(id string, data []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, exists := c.entries[id]; !exists {
		if len(c.entries) >= c.maxSize {
			oldest := c.order[0]
			c.order = c.order[1:]
			delete(c.entries, oldest)
		}
		c.order = append(c.order, id)
	}
	c.entries[id] = data
}

func (c *VehicleCache) runReset(ctx context.Context) {
	ticker := time.NewTicker(30 * 24 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.mu.Lock()
			c.entries = make(map[string][]byte)
			c.order = c.order[:0]
			c.mu.Unlock()
		}
	}
}

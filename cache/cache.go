package cache

import (
	"context"
	"sync"
	"time"
)

// EmailCache speichert validierte E-Mail-Adressen mit einem Zeitstempel.
// Einträge älter als TTL werden periodisch bereinigt.
type EmailCache struct {
	entries sync.Map
	ttl     time.Duration
}

// NewEmailCache erstellt einen neuen EmailCache und startet die Cleanup-Goroutine.
// Die Goroutine läuft alle 10 Minuten und stoppt wenn ctx cancelled wird.
func NewEmailCache(ttl time.Duration, ctx context.Context) *EmailCache {
	c := &EmailCache{ttl: ttl}
	go c.runCleanup(ctx)
	return c
}

// IsVerified gibt true zurück wenn die E-Mail im Cache vorhanden ist.
// Es wird kein TTL-Check beim Lesen durchgeführt.
func (c *EmailCache) IsVerified(email string) bool {
	_, ok := c.entries.Load(email)
	return ok
}

// Add speichert die E-Mail mit dem aktuellen Zeitstempel im Cache.
func (c *EmailCache) Add(email string) {
	c.entries.Store(email, time.Now())
}

func (c *EmailCache) runCleanup(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			cutoff := time.Now().Add(-c.ttl)
			c.entries.Range(func(key, value any) bool {
				if value.(time.Time).Before(cutoff) {
					c.entries.Delete(key)
				}
				return true
			})
		}
	}
}

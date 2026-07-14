// Package cache provides a simple TTL-based in-memory cache for frequently
// accessed data such as repo and user lookups.
package cache

import (
	"sync"
	"time"
)

type entry struct {
	value   any
	expires time.Time
}

// Cache is a goroutine-safe TTL-based in-memory cache.
type Cache struct {
	mu       sync.RWMutex
	entries  map[string]*entry
	ttl      time.Duration
	stopCh   chan struct{}
	stopOnce sync.Once
}

// New creates a new Cache with the given TTL. It starts a background
// goroutine that evicts expired entries every cleanupInterval.
func New(ttl time.Duration) *Cache {
	c := &Cache{
		entries: make(map[string]*entry),
		ttl:     ttl,
		stopCh:  make(chan struct{}),
	}
	go c.evictLoop(time.Minute)
	return c
}

// Get returns the value for key and whether it was found.
func (c *Cache) Get(key string) (any, bool) {
	c.mu.RLock()
	e, ok := c.entries[key]
	c.mu.RUnlock()
	if !ok {
		return nil, false
	}
	if time.Now().After(e.expires) {
		c.Delete(key)
		return nil, false
	}
	return e.value, true
}

// Set stores a value under key with the cache's TTL.
func (c *Cache) Set(key string, value any) {
	c.mu.Lock()
	c.entries[key] = &entry{value: value, expires: time.Now().Add(c.ttl)}
	c.mu.Unlock()
}

// Delete removes a key from the cache.
func (c *Cache) Delete(key string) {
	c.mu.Lock()
	delete(c.entries, key)
	c.mu.Unlock()
}

// Clear removes all entries from the cache.
func (c *Cache) Clear() {
	c.mu.Lock()
	c.entries = make(map[string]*entry)
	c.mu.Unlock()
}

// Stop terminates the background eviction goroutine.
func (c *Cache) Stop() {
	c.stopOnce.Do(func() {
		close(c.stopCh)
	})
}

func (c *Cache) evictLoop(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			c.evictExpired()
		case <-c.stopCh:
			return
		}
	}
}

func (c *Cache) evictExpired() {
	now := time.Now()
	c.mu.Lock()
	defer c.mu.Unlock()
	for k, e := range c.entries {
		if now.After(e.expires) {
			delete(c.entries, k)
		}
	}
}

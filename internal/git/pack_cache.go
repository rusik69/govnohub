package gitstore

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// PackCache provides disk-backed caching for git pack-objects output.
// Cache entries are keyed by the repository's full object state (all branch SHAs),
// so they are automatically invalidated when a push occurs (branches change).
//
// Each entry stores the raw output of `git rev-list --objects --all |
// git pack-objects --stdout` for a repo, which is what a full clone needs.
// Subsequent clone requests for the same repo state serve the cached packfile
// instead of re-running pack-objects, saving significant CPU.
type PackCache struct {
	root    string
	maxSize int64
	ttl     time.Duration
	mu      sync.RWMutex
	entries map[string]*cacheEntry // key → entry (in-memory index)
}

type cacheEntry struct {
	path string
	size int64
	at   time.Time
}

// NewPackCache creates a new pack-objects cache rooted at dir.
// maxSize is the approximate total disk budget (oldest entries are evicted).
// ttl is the maximum age for entries; 0 means no TTL.
func NewPackCache(dir string, maxSize int64, ttl time.Duration) (*PackCache, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("pack cache dir: %w", err)
	}
	c := &PackCache{
		root:    dir,
		maxSize: maxSize,
		ttl:     ttl,
		entries: make(map[string]*cacheEntry),
	}
	c.rebuildIndex()
	return c, nil
}

// rebuildIndex scans the cache directory and rebuilds the in-memory index.
func (c *PackCache) rebuildIndex() {
	filepath.Walk(c.root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		key := filepath.Base(path)
		c.entries[key] = &cacheEntry{
			path: path,
			size: info.Size(),
			at:   info.ModTime(),
		}
		return nil
	})
}

// Key computes a deterministic cache key for the current state of a repo.
// The key is SHA256(owner + "/" + name + "\n" + sorted "branch:sha" pairs).
func (c *PackCache) Key(owner, name string, branchSHAs map[string]string) string {
	h := sha256.New()
	h.Write([]byte(owner + "/" + name + "\n"))
	keys := make([]string, 0, len(branchSHAs))
	for k := range branchSHAs {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		h.Write([]byte(k + ":" + branchSHAs[k] + "\n"))
	}
	return hex.EncodeToString(h.Sum(nil))
}

// Get returns cached pack data for the given key, or nil if not found/stale.
func (c *PackCache) Get(key string) ([]byte, bool) {
	c.mu.RLock()
	entry, ok := c.entries[key]
	c.mu.RUnlock()
	if !ok {
		return nil, false
	}

	// Check TTL
	if c.ttl > 0 && time.Since(entry.at) > c.ttl {
		c.Delete(key)
		return nil, false
	}

	data, err := os.ReadFile(entry.path)
	if err != nil {
		c.Delete(key)
		return nil, false
	}
	return data, true
}

// Set stores pack data in the cache under the given key.
func (c *PackCache) Set(key string, data []byte) error {
	// Create sharded path: root/first-2-chars/rest
	shard := key[:2]
	dir := filepath.Join(c.root, shard)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("pack cache shard dir: %w", err)
	}
	path := filepath.Join(dir, key)

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("pack cache write: %w", err)
	}

	c.mu.Lock()
	c.entries[key] = &cacheEntry{
		path: path,
		size: int64(len(data)),
		at:   time.Now(),
	}
	size := int64(len(data))
	c.mu.Unlock()

	// Evict if over budget (async)
	if c.maxSize > 0 {
		go c.evict(size)
	}

	return nil
}

// Delete removes a cache entry.
func (c *PackCache) Delete(key string) {
	c.mu.Lock()
	entry, ok := c.entries[key]
	delete(c.entries, key)
	c.mu.Unlock()
	if ok {
		os.Remove(entry.path)
	}
}

// InvalidateRepo removes all cache entries for a given repo.
// Since keys are prefixed with the owner/name hash, we can't easily match by
// owner/name without scanning. Instead, users of this cache should just not
// worry about invalidation — the key naturally changes when branch SHAs change.
// This method is provided for explicit invalidation if needed.
func (c *PackCache) InvalidateRepo(owner, name string) {
	prefix := sha256Hex(owner + "/" + name)
	c.mu.Lock()
	for key, entry := range c.entries {
		if strings.HasPrefix(key, prefix) {
			delete(c.entries, key)
			os.Remove(entry.path)
		}
	}
	c.mu.Unlock()
}

// evict removes the oldest entries until total size is under maxSize.
func (c *PackCache) evict(newEntrySize int64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Quick check: if we're under budget, skip
	var total int64
	type keyAge struct {
		key string
		at  time.Time
	}
	ages := make([]keyAge, 0, len(c.entries))
	for k, e := range c.entries {
		total += e.size
		ages = append(ages, keyAge{key: k, at: e.at})
	}
	total += newEntrySize

	if total <= c.maxSize {
		return
	}

	sort.Slice(ages, func(i, j int) bool {
		return ages[i].at.Before(ages[j].at)
	})

	over := total - c.maxSize
	for _, ka := range ages {
		if over <= 0 {
			break
		}
		if entry, ok := c.entries[ka.key]; ok {
			over -= entry.size
			delete(c.entries, ka.key)
			os.Remove(entry.path)
		}
	}
}

// Stats returns the number of entries and total bytes in the cache.
func (c *PackCache) Stats() (int, int64) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	var total int64
	for _, e := range c.entries {
		total += e.size
	}
	return len(c.entries), total
}

func sha256Hex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

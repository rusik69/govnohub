package gitstore

import (
	"os"
	"testing"
	"time"
)

func TestPackCacheKey(t *testing.T) {
	c, err := NewPackCache(t.TempDir(), 1<<30, 0)
	if err != nil {
		t.Fatal(err)
	}

	// Deterministic key for same input
	branches := map[string]string{
		"main":  "abc123",
		"feat":  "def456",
	}
	k1 := c.Key("alice", "repo1", branches)
	k2 := c.Key("alice", "repo1", branches)
	if k1 != k2 {
		t.Fatal("keys should be deterministic")
	}

	// Different repo → different key
	k3 := c.Key("bob", "repo1", branches)
	if k1 == k3 {
		t.Fatal("different owner should produce different key")
	}

	// Different branch state → different key
	branches2 := map[string]string{
		"main":  "xyz789",
	}
	k4 := c.Key("alice", "repo1", branches2)
	if k1 == k4 {
		t.Fatal("different branch SHAs should produce different key")
	}

	// Same branches in different order → same key
	k5 := c.Key("alice", "repo1", map[string]string{
		"feat":  "def456",
		"main":  "abc123",
	})
	if k1 != k5 {
		t.Fatal("key should be order-independent (sorted keys)")
	}
}

func TestPackCacheSetGet(t *testing.T) {
	c, err := NewPackCache(t.TempDir(), 1<<30, 0)
	if err != nil {
		t.Fatal(err)
	}

	key := "testkey123"
	data := []byte("packfile contents")
	if err := c.Set(key, data); err != nil {
		t.Fatal(err)
	}

	got, ok := c.Get(key)
	if !ok {
		t.Fatal("expected cache hit")
	}
	if string(got) != string(data) {
		t.Fatalf("got %q, want %q", got, data)
	}
}

func TestPackCacheMiss(t *testing.T) {
	c, err := NewPackCache(t.TempDir(), 1<<30, 0)
	if err != nil {
		t.Fatal(err)
	}

	_, ok := c.Get("nonexistent")
	if ok {
		t.Fatal("expected cache miss")
	}
}

func TestPackCacheDelete(t *testing.T) {
	c, err := NewPackCache(t.TempDir(), 1<<30, 0)
	if err != nil {
		t.Fatal(err)
	}

	key := "deletekey"
	if err := c.Set(key, []byte("data")); err != nil {
		t.Fatal(err)
	}

	c.Delete(key)
	_, ok := c.Get(key)
	if ok {
		t.Fatal("expected cache miss after delete")
	}
}

func TestPackCacheTTL(t *testing.T) {
	c, err := NewPackCache(t.TempDir(), 1<<30, 1*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}

	key := "ttlkey"
	if err := c.Set(key, []byte("data")); err != nil {
		t.Fatal(err)
	}

	// Should be immediately available
	if _, ok := c.Get(key); !ok {
		t.Fatal("expected cache hit before TTL expiry")
	}

	// Wait for TTL to expire
	time.Sleep(2 * time.Millisecond)
	if _, ok := c.Get(key); ok {
		t.Fatal("expected cache miss after TTL expiry")
	}
}

func TestPackCacheEviction(t *testing.T) {
	// Create a cache with max size just enough for one entry
	c, err := NewPackCache(t.TempDir(), 10, 0) // 10 bytes max
	if err != nil {
		t.Fatal(err)
	}
	defer c.evict(0) //nolint:errcheck // just cleanup

	// First entry fits
	if err := c.Set("key1", []byte("12345")); err != nil { // 5 bytes
		t.Fatal(err)
	}

	// Second entry triggers eviction of the first
	if err := c.Set("key2", []byte("67890")); err != nil { // 5 bytes
		t.Fatal(err)
	}

	// Wait for async eviction
	time.Sleep(50 * time.Millisecond)

	_, ok1 := c.Get("key1")
	_, ok2 := c.Get("key2")
	if ok1 {
		t.Fatal("expected key1 to be evicted")
	}
	if !ok2 {
		t.Fatal("expected key2 to remain")
	}
}

func TestPackCacheStats(t *testing.T) {
	c, err := NewPackCache(t.TempDir(), 1<<30, 0)
	if err != nil {
		t.Fatal(err)
	}

	count, total := c.Stats()
	if count != 0 || total != 0 {
		t.Fatalf("expected empty stats, got %d entries, %d bytes", count, total)
	}

	c.Set("k1", []byte("data1"))
	c.Set("k2", []byte("data2 larger"))

	count, total = c.Stats()
	if count != 2 {
		t.Fatalf("expected 2 entries, got %d", count)
	}
	if total == 0 {
		t.Fatal("expected non-zero total bytes")
	}
}

func TestPackCacheRebuildIndex(t *testing.T) {
	tmp := t.TempDir()
	_, err := NewPackCache(tmp, 1<<30, 0)
	if err != nil {
		t.Fatal(err)
	}

	// Add an entry directly on disk
	shard := tmp + "/ab"
	if err := os.MkdirAll(shard, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(shard+"/abcdef", []byte("cached"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Rebuild index and check
	c2, err := NewPackCache(tmp, 1<<30, 0)
	if err != nil {
		t.Fatal(err)
	}

	data, ok := c2.Get("abcdef")
	if !ok {
		t.Fatal("expected to find entry after rebuild")
	}
	if string(data) != "cached" {
		t.Fatalf("got %q, want %q", data, "cached")
	}
}

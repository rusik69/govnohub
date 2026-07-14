package cache

import (
	"sync"
	"testing"
	"time"
)

func TestCacheGetSet(t *testing.T) {
	c := New(time.Minute)
	defer c.Stop()

	c.Set("key1", "value1")
	v, ok := c.Get("key1")
	if !ok {
		t.Fatal("expected key1 to be found")
	}
	if v.(string) != "value1" {
		t.Fatalf("expected %q, got %q", "value1", v.(string))
	}
}

func TestCacheGetMissing(t *testing.T) {
	c := New(time.Minute)
	defer c.Stop()

	_, ok := c.Get("nonexistent")
	if ok {
		t.Fatal("expected nonexistent key to not be found")
	}
}

func TestCacheDelete(t *testing.T) {
	c := New(time.Minute)
	defer c.Stop()

	c.Set("key1", "value1")
	c.Delete("key1")
	_, ok := c.Get("key1")
	if ok {
		t.Fatal("expected key1 to be deleted")
	}
}

func TestCacheClear(t *testing.T) {
	c := New(time.Minute)
	defer c.Stop()

	c.Set("a", 1)
	c.Set("b", 2)
	c.Clear()

	_, ok := c.Get("a")
	if ok {
		t.Fatal("expected cache to be empty after clear")
	}
	_, ok = c.Get("b")
	if ok {
		t.Fatal("expected cache to be empty after clear")
	}
}

func TestCacheExpiry(t *testing.T) {
	c := New(50 * time.Millisecond)
	defer c.Stop()

	c.Set("key1", "value1")
	time.Sleep(100 * time.Millisecond)
	_, ok := c.Get("key1")
	if ok {
		t.Fatal("expected key1 to have expired")
	}

	// Verify a non-expired entry still works
	c.Set("key2", "value2")
	v, ok := c.Get("key2")
	if !ok {
		t.Fatal("expected key2 to be found")
	}
	if v.(string) != "value2" {
		t.Fatalf("expected %q, got %q", "value2", v.(string))
	}
}

func TestCacheConcurrentAccess(t *testing.T) {
	c := New(time.Minute)
	defer c.Stop()

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			key := "key:" + string(rune(i))
			c.Set(key, i)
			v, ok := c.Get(key)
			if ok && v.(int) != i {
				t.Errorf("unexpected value for %s: got %d, want %d", key, v.(int), i)
			}
		}(i)
	}
	wg.Wait()
}

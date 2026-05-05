// go/internal/services/cache_test.go
package services

import (
	"testing"
	"time"
)

func TestCacheGetSet(t *testing.T) {
	cache := NewLRUCache[string, string](2, 1*time.Hour)

	cache.Set("key1", "value1")
	cache.Set("key2", "value2")

	val, ok := cache.Get("key1")
	if !ok || val != "value1" {
		t.Errorf("expected value1, got %s, ok=%v", val, ok)
	}

	val, ok = cache.Get("key2")
	if !ok || val != "value2" {
		t.Errorf("expected value2, got %s, ok=%v", val, ok)
	}
}

func TestCacheEviction(t *testing.T) {
	cache := NewLRUCache[string, string](2, 1*time.Hour)

	cache.Set("key1", "value1")
	cache.Set("key2", "value2")
	cache.Set("key3", "value3") // Should evict key1

	_, ok := cache.Get("key1")
	if ok {
		t.Error("expected key1 to be evicted")
	}

	val, ok := cache.Get("key2")
	if !ok || val != "value2" {
		t.Errorf("expected value2, got %s, ok=%v", val, ok)
	}

	val, ok = cache.Get("key3")
	if !ok || val != "value3" {
		t.Errorf("expected value3, got %s, ok=%v", val, ok)
	}
}

func TestCacheTTL(t *testing.T) {
	cache := NewLRUCache[string, string](10, 100*time.Millisecond)

	cache.Set("key1", "value1")

	val, ok := cache.Get("key1")
	if !ok || val != "value1" {
		t.Errorf("expected value1, got %s, ok=%v", val, ok)
	}

	time.Sleep(150 * time.Millisecond)

	_, ok = cache.Get("key1")
	if ok {
		t.Error("expected key1 to be expired")
	}
}

func TestCacheStats(t *testing.T) {
	cache := NewLRUCache[string, string](10, 1*time.Hour)

	cache.Set("key1", "value1")
	cache.Get("key1") // hit
	cache.Get("key2") // miss
	cache.Get("key1") // hit

	stats := cache.Stats()
	if stats.Hits != 2 {
		t.Errorf("expected 2 hits, got %d", stats.Hits)
	}
	if stats.Misses != 1 {
		t.Errorf("expected 1 miss, got %d", stats.Misses)
	}
}
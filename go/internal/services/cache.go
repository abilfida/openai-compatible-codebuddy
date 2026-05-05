// go/internal/services/cache.go
package services

import (
	"container/list"
	"sync"
	"time"
)

type CacheStats struct {
	Hits    int
	Misses  int
	Size    int
	HitRate float64
}

type cacheEntry[V any] struct {
	key       string
	value     V
	expiresAt time.Time
}

type LRUCache[K comparable, V any] struct {
	maxSize int
	ttl     time.Duration
	entries map[string]*list.Element
	order   *list.List
	mu      sync.RWMutex
	hits    int
	misses  int
}

func NewLRUCache[K comparable, V any](maxSize int, ttl time.Duration) *LRUCache[K, V] {
	return &LRUCache[K, V]{
		maxSize: maxSize,
		ttl:     ttl,
		entries: make(map[string]*list.Element),
		order:   list.New(),
	}
}

func (c *LRUCache[K, V]) Get(key string) (V, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	var zero V

	elem, ok := c.entries[key]
	if !ok {
		c.misses++
		return zero, false
	}

	entry := elem.Value.(*cacheEntry[V])
	if time.Now().After(entry.expiresAt) {
		c.order.Remove(elem)
		delete(c.entries, key)
		c.misses++
		return zero, false
	}

	// Move to front (most recently used)
	c.order.MoveToFront(elem)
	c.hits++
	return entry.value, true
}

func (c *LRUCache[K, V]) Set(key string, value V) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Remove existing if present
	if elem, ok := c.entries[key]; ok {
		c.order.Remove(elem)
		delete(c.entries, key)
	}

	// Evict oldest if at capacity
	if c.order.Len() >= c.maxSize {
		oldest := c.order.Back()
		if oldest != nil {
			c.order.Remove(oldest)
			oldEntry := oldest.Value.(*cacheEntry[V])
			delete(c.entries, oldEntry.key)
		}
	}

	entry := &cacheEntry[V]{
		key:       key,
		value:     value,
		expiresAt: time.Now().Add(c.ttl),
	}
	elem := c.order.PushFront(entry)
	c.entries[key] = elem
}

func (c *LRUCache[K, V]) Stats() CacheStats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	total := c.hits + c.misses
	hitRate := 0.0
	if total > 0 {
		hitRate = float64(c.hits) / float64(total)
	}

	return CacheStats{
		Hits:    c.hits,
		Misses:  c.misses,
		Size:    c.order.Len(),
		HitRate: hitRate,
	}
}
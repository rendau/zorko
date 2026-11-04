package service

import (
	"container/list"
	"sync"
	"time"
)

// lru — разобранные файлы в памяти с пределом по байтам; вытесняются самые давние
type lru struct {
	mu    sync.Mutex
	limit int64
	size  int64
	ll    *list.List
	items map[string]*list.Element
}

type lruEntry struct {
	key   string
	value any
	size  int64
}

func newLru(limit int64) *lru {
	return &lru{limit: limit, ll: list.New(), items: map[string]*list.Element{}}
}

func (c *lru) get(key string) (any, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.items[key]; ok {
		c.ll.MoveToFront(el)
		return el.Value.(*lruEntry).value, true
	}
	return nil, false
}

func (c *lru) put(key string, value any, size int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.items[key]; ok {
		c.size -= el.Value.(*lruEntry).size
		c.ll.Remove(el)
	}
	if size > c.limit {
		return // больше всего кэша — не держим
	}
	c.items[key] = c.ll.PushFront(&lruEntry{key: key, value: value, size: size})
	c.size += size
	for c.size > c.limit {
		el := c.ll.Back()
		e := el.Value.(*lruEntry)
		c.ll.Remove(el)
		delete(c.items, e.key)
		c.size -= e.size
	}
	metricCacheBytes.Set(float64(c.size))
}

// ttlCache — результаты поиска карт на короткое время (карту, залитую чуть позже, подхватит быстро)
type ttlCache[V any] struct {
	mu    sync.Mutex
	ttl   time.Duration
	max   int
	items map[string]ttlItem[V]
}

type ttlItem[V any] struct {
	value   V
	expires time.Time
}

func newTtlCache[V any](ttl time.Duration, maxItems int) *ttlCache[V] {
	return &ttlCache[V]{ttl: ttl, max: maxItems, items: map[string]ttlItem[V]{}}
}

func (c *ttlCache[V]) get(key string) (V, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	it, ok := c.items[key]
	if !ok || time.Now().After(it.expires) {
		var zero V
		return zero, false
	}
	return it.value, true
}

func (c *ttlCache[V]) put(key string, v V) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.items) >= c.max {
		now := time.Now()
		for k, it := range c.items {
			if now.After(it.expires) {
				delete(c.items, k)
			}
		}
		if len(c.items) >= c.max {
			c.items = map[string]ttlItem[V]{}
		}
	}
	c.items[key] = ttlItem[V]{value: v, expires: time.Now().Add(c.ttl)}
}

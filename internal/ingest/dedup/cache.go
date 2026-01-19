package dedup

import (
	"container/list"
	"sync"
	"time"
)

type Cache struct {
	mu         sync.Mutex
	maxEntries int
	ttl        time.Duration
	items      map[string]*list.Element
	order      *list.List
}

type entry struct {
	key       string
	expiresAt time.Time
}

func NewCache(maxEntries int, ttl time.Duration) *Cache {
	if maxEntries <= 0 {
		maxEntries = 100000
	}
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}
	return &Cache{
		maxEntries: maxEntries,
		ttl:        ttl,
		items:      make(map[string]*list.Element),
		order:      list.New(),
	}
}

// Seen returns true if key exists and is not expired. Otherwise it records it.
func (c *Cache) Seen(key string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	if element, ok := c.items[key]; ok {
		ent := element.Value.(*entry)
		if ent.expiresAt.After(now) {
			ent.expiresAt = now.Add(c.ttl)
			c.order.MoveToFront(element)
			return true
		}
		c.removeElement(element)
	}

	ent := &entry{
		key:       key,
		expiresAt: now.Add(c.ttl),
	}
	element := c.order.PushFront(ent)
	c.items[key] = element

	if c.maxEntries > 0 && c.order.Len() > c.maxEntries {
		c.removeOldest()
	}

	return false
}

func (c *Cache) removeOldest() {
	element := c.order.Back()
	if element != nil {
		c.removeElement(element)
	}
}

func (c *Cache) removeElement(element *list.Element) {
	c.order.Remove(element)
	ent := element.Value.(*entry)
	delete(c.items, ent.key)
}

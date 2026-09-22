package cache

import (
	"sync"
	"time"
)

// item 缓存条目
type item struct {
	value      interface{}
	expireTime time.Time
}

// Cache 内存缓存实例，线程安全
type Cache struct {
	mu    sync.RWMutex
	items map[string]*item
}

// NewCache 初始化缓存
func NewCache() *Cache {
	c := &Cache{
		items: make(map[string]*item),
	}
	// 启动后台清理过期缓存协程
	go c.cleanupLoop()
	return c
}

// Set 设置缓存，expire为过期时长
func (c *Cache) Set(key string, value interface{}, expire time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items[key] = &item{
		value:      value,
		expireTime: time.Now().Add(expire),
	}
}

// Get 获取缓存，命中返回值和true，未命中/过期返回nil和false
func (c *Cache) Get(key string) (interface{}, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	it, ok := c.items[key]
	if !ok {
		return nil, false
	}
	// 过期直接返回未命中
	if time.Now().After(it.expireTime) {
		return nil, false
	}
	return it.value, true
}

// Delete 删除指定缓存
func (c *Cache) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.items, key)
}

// cleanupLoop 后台定期清理过期缓存，每10分钟执行一次
func (c *Cache) cleanupLoop() {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		c.mu.Lock()
		now := time.Now()
		for k, v := range c.items {
			if now.After(v.expireTime) {
				delete(c.items, k)
			}
		}
		c.mu.Unlock()
	}
}

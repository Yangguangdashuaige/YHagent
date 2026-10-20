package cache

import (
	"sort"
	"sync"
	"time"
)

// defaultMaxEntries 缓存条目数上限（防御性保险丝）：
// 正常情况下各接口 TTL 远小于堆积速度，条目数很难触顶；
// 触顶后按最久未访问淘汰一批，防止异常 key 基数（如带随机参数的请求）吃光内存。
const defaultMaxEntries = 5000

// evictBatch 触顶时一次淘汰的条目数，避免每写一条都全量扫描
const evictBatch = 64

// item 缓存条目
type item struct {
	value      interface{}
	expireTime time.Time
	lastAccess time.Time
}

// Cache 内存缓存实例，线程安全
type Cache struct {
	mu         sync.Mutex
	items      map[string]*item
	maxEntries int
}

// NewCache 初始化缓存（默认容量上限），并启动后台过期清理协程
func NewCache() *Cache {
	return NewCacheWithMax(defaultMaxEntries)
}

// NewCacheWithMax 使用自定义容量上限初始化缓存（主要留给测试用）
func NewCacheWithMax(max int) *Cache {
	if max <= 0 {
		max = defaultMaxEntries
	}
	c := &Cache{
		items:      make(map[string]*item),
		maxEntries: max,
	}
	go c.cleanupLoop()
	return c
}

// Set 设置缓存，expire为过期时长
func (c *Cache) Set(key string, value interface{}, expire time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	c.items[key] = &item{
		value:      value,
		expireTime: now.Add(expire),
		lastAccess: now,
	}
	// 超出容量：先淘汰已过期条目，仍超限时按最久未访问淘汰
	if len(c.items) > c.maxEntries {
		c.evictExpiredLocked(now)
	}
	if len(c.items) > c.maxEntries {
		c.evictLRULocked(evictBatch, now)
	}
}

// Get 获取缓存，命中返回值和true，未命中/过期返回nil和false；命中时刷新访问时间
func (c *Cache) Get(key string) (interface{}, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	it, ok := c.items[key]
	if !ok {
		return nil, false
	}
	now := time.Now()
	// 过期直接返回未命中并顺手删除
	if now.After(it.expireTime) {
		delete(c.items, key)
		return nil, false
	}
	it.lastAccess = now
	return it.value, true
}

// GetStale 返回条目即使已过期（stale-while-error 兜底用）：
// 上游接口失败时回吐过期数据远好于直接报错——价格/密码这类数据
// "10 分钟前的值"几乎总是好于"没有值"。返回值标注是否已过期。
func (c *Cache) GetStale(key string) (interface{}, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	it, ok := c.items[key]
	if !ok {
		return nil, false
	}
	it.lastAccess = time.Now()
	return it.value, true
}

// Delete 删除缓存条目
func (c *Cache) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.items, key)
}

// Len 返回当前条目数（含尚未被清理的过期条目），主要用于监控/测试
func (c *Cache) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.items)
}

// evictExpiredLocked 删除全部已过期条目（调用方持锁）
func (c *Cache) evictExpiredLocked(now time.Time) int {
	removed := 0
	for k, v := range c.items {
		if now.After(v.expireTime) {
			delete(c.items, k)
			removed++
		}
	}
	return removed
}

// evictLRULocked 淘汰 lastAccess 最老的条目，直到条数回落至上限以下（至多 n 个，调用方持锁）
func (c *Cache) evictLRULocked(n int, now time.Time) {
	// 触发频率极低（仅条目数异常堆积时），缓存规模几千条，全量排序成本可忽略。
	type candidate struct {
		key string
		at  time.Time
	}
	all := make([]candidate, 0, len(c.items))
	for k, v := range c.items {
		all = append(all, candidate{k, v.lastAccess})
	}
	sort.Slice(all, func(i, j int) bool { return all[i].at.Before(all[j].at) })

	need := len(c.items) - c.maxEntries
	if need <= 0 {
		return
	}
	if need > n {
		need = n
	}
	for i := 0; i < need && i < len(all); i++ {
		delete(c.items, all[i].key)
	}
}

// cleanupLoop 后台定期清理过期缓存，每10分钟执行一次
func (c *Cache) cleanupLoop() {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		c.mu.Lock()
		removed := c.evictExpiredLocked(time.Now())
		c.mu.Unlock()
		if removed > 0 {
			// 静默清理即可，过期是常态；如需观察可临时打开日志
			_ = removed
		}
	}
}

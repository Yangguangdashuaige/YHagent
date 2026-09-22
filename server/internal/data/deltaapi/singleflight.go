// internal/data/deltaapi/singleflight.go
//
// 极简 singleflight（仅用标准库实现，不引入 golang.org/x/sync）：
// 同一时刻相同 key 的并发调用只会真正执行一次，其余调用等待并共享同一份结果。
// 服务端化以后，缓存恰好过期的瞬间即使有几百个玩家同时查询同一物品，
// 上游三角洲接口也只会被打到一次，避免缓存击穿浪费接口资源点。
package deltaapi

import (
	"encoding/json"
	"sync"
)

type flightCall struct {
	done chan struct{}
	val  json.RawMessage
	err  error
}

type flightGroup struct {
	mu sync.Mutex
	m  map[string]*flightCall
}

// Do 同 key 并发合并：首个调用执行 fn 并共享结果（含错误），后续调用阻塞等待
func (g *flightGroup) Do(key string, fn func() (json.RawMessage, error)) (json.RawMessage, error) {
	g.mu.Lock()
	if g.m == nil {
		g.m = make(map[string]*flightCall)
	}
	if c, ok := g.m[key]; ok {
		g.mu.Unlock()
		<-c.done
		return c.val, c.err
	}
	c := &flightCall{done: make(chan struct{})}
	g.m[key] = c
	g.mu.Unlock()

	c.val, c.err = fn()
	close(c.done)

	g.mu.Lock()
	delete(g.m, key)
	g.mu.Unlock()
	return c.val, c.err
}

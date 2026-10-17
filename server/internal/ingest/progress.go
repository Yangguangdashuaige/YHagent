// Package ingest 的入库进度回调：
// Service.Ingest 通过 context 把阶段事件传给上层（server 经 SSE 推给网页端），
// 与 websearch 候选卡片的 WithCandidateFunc 同一套思路，避免在低层包依赖 HTTP。
package ingest

import (
	"context"
	"strings"
	"time"
)

// 进度阶段名（前端按此渲染步骤条）
const (
	StageOrganizing = "organizing" // AI 开始整理
	StageWriting    = "writing"    // AI 正在逐字撰写（Delta 为本次增量）
	StageChunking   = "chunking"   // Markdown 切片
	StageEmbedding  = "embedding"  // 分批向量化（Done/Total）
)

// Progress 一次入库进度事件
type Progress struct {
	Stage string `json:"stage"`
	Done  int    `json:"done,omitempty"`
	Total int    `json:"total,omitempty"`
	Delta string `json:"delta,omitempty"`
}

// ProgressFunc 进度回调（由装配层注入，内部直接写 SSE）
type ProgressFunc func(Progress)

type progressKey struct{}

// WithProgress 把进度回调挂到 context；f 为 nil 时原样返回
func WithProgress(ctx context.Context, f ProgressFunc) context.Context {
	if f == nil {
		return ctx
	}
	return context.WithValue(ctx, progressKey{}, f)
}

func emitProgress(ctx context.Context, p Progress) {
	if f, ok := ctx.Value(progressKey{}).(ProgressFunc); ok && f != nil {
		f(p)
	}
}

// deltaFlushInterval AI 逐字输出的 SSE 节流间隔：
// 模型一个 token 一个回调，全部直推会产生上千个事件，按时间+长度合并即可。
const deltaFlushInterval = 120 * time.Millisecond
const deltaFlushChars = 48

// deltaWriter 把 AI 输出增量节流合并后通过进度回调推给前端（单 goroutine 使用）
type deltaWriter struct {
	ctx  context.Context
	buf  strings.Builder
	last time.Time
}

func newDeltaWriter(ctx context.Context) *deltaWriter {
	return &deltaWriter{ctx: ctx}
}

func (w *deltaWriter) push(s string) {
	if s == "" {
		return
	}
	w.buf.WriteString(s)
	if time.Since(w.last) >= deltaFlushInterval || w.buf.Len() >= deltaFlushChars {
		w.flush()
	}
}

func (w *deltaWriter) flush() {
	if w.buf.Len() == 0 {
		return
	}
	emitProgress(w.ctx, Progress{Stage: StageWriting, Delta: w.buf.String()})
	w.buf.Reset()
	w.last = time.Now()
}

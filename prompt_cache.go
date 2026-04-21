package ragagent

import (
	"context"
	"sync"

	"github.com/gtkit/go-rag-agent/internal/graph"
	"github.com/gtkit/go-rag-agent/internal/llm"
)

// PromptCache 定义 prompt 构造结果缓存能力。
type PromptCache interface {
	Get(ctx context.Context, key string) ([]Message, bool)
	Set(ctx context.Context, key string, messages []Message)
}

type rootToGraphPromptCache struct {
	inner PromptCache
}

func (c rootToGraphPromptCache) Get(ctx context.Context, key string) ([]llm.Message, bool) {
	if c.inner == nil {
		return nil, false
	}
	msgs, ok := c.inner.Get(ctx, key)
	return msgs, ok
}

func (c rootToGraphPromptCache) Set(ctx context.Context, key string, messages []llm.Message) {
	if c.inner == nil {
		return
	}
	c.inner.Set(ctx, key, messages)
}

type inMemoryPromptCache struct {
	mu    sync.RWMutex
	items map[string][]Message
}

// NewInMemoryPromptCache 创建默认内存 prompt cache。
func NewInMemoryPromptCache() PromptCache {
	return &inMemoryPromptCache{
		items: make(map[string][]Message),
	}
}

func (c *inMemoryPromptCache) Get(_ context.Context, key string) ([]Message, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	msgs, ok := c.items[key]
	if !ok {
		return nil, false
	}
	return append([]Message(nil), msgs...), true
}

func (c *inMemoryPromptCache) Set(_ context.Context, key string, messages []Message) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items[key] = append([]Message(nil), messages...)
}

var _ graph.PromptCache = rootToGraphPromptCache{}

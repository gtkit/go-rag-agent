package ragagent

import (
	"container/list"
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/gtkit/go-rag-agent/internal/graph"
	"github.com/gtkit/go-rag-agent/internal/llm"
	"github.com/redis/go-redis/v9"
)

// PromptCache 定义 prompt 构造结果缓存能力。
type PromptCache interface {
	Get(ctx context.Context, key string) ([]Message, bool)
	Set(ctx context.Context, key string, messages []Message)
}

// RedisPromptCacheConfig 定义 Redis prompt cache 配置。
type RedisPromptCacheConfig struct {
	KeyPrefix string
	TTL       time.Duration
}

func (c RedisPromptCacheConfig) normalized() RedisPromptCacheConfig {
	if c.KeyPrefix == "" {
		c.KeyPrefix = "ragagent:prompt"
	}
	return c
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
	mu    sync.Mutex
	items map[string]*list.Element
	lru   *list.List
	cfg   PromptCacheConfig
	now   func() time.Time
}

type redisPromptCache struct {
	client redis.Cmdable
	cfg    RedisPromptCacheConfig
}

type twoLevelPromptCache struct {
	local  PromptCache
	remote PromptCache
}

// PromptCacheConfig 定义有界 prompt cache 的配置。
type PromptCacheConfig struct {
	MaxEntries int
	TTL        time.Duration
}

func (c PromptCacheConfig) normalized() PromptCacheConfig {
	if c.MaxEntries <= 0 {
		c.MaxEntries = 1024
	}
	return c
}

type promptCacheEntry struct {
	key       string
	messages  []Message
	expiresAt time.Time
}

// NewInMemoryPromptCache 创建默认内存 prompt cache。
func NewInMemoryPromptCache() PromptCache {
	return NewInMemoryPromptCacheWithConfig(PromptCacheConfig{})
}

// NewInMemoryPromptCacheWithConfig 创建可配置的有界内存 prompt cache。
func NewInMemoryPromptCacheWithConfig(cfg PromptCacheConfig) PromptCache {
	cfg = cfg.normalized()
	return &inMemoryPromptCache{
		items: make(map[string]*list.Element),
		lru:   list.New(),
		cfg:   cfg,
		now:   time.Now,
	}
}

// NewRedisPromptCache 创建 Redis prompt cache。
func NewRedisPromptCache(client redis.Cmdable, cfg RedisPromptCacheConfig) PromptCache {
	if client == nil {
		return nil
	}
	return &redisPromptCache{
		client: client,
		cfg:    cfg.normalized(),
	}
}

// NewTwoLevelPromptCache 创建本地 + 远端的两级 prompt cache。
func NewTwoLevelPromptCache(local PromptCache, remote PromptCache) PromptCache {
	return &twoLevelPromptCache{
		local:  local,
		remote: remote,
	}
}

func (c *inMemoryPromptCache) Get(_ context.Context, key string) ([]Message, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	elem, ok := c.items[key]
	if !ok {
		return nil, false
	}
	entry := elem.Value.(*promptCacheEntry)
	if c.expired(entry) {
		c.removeElement(elem)
		return nil, false
	}
	c.lru.MoveToFront(elem)
	return append([]Message(nil), entry.messages...), true
}

func (c *inMemoryPromptCache) Set(_ context.Context, key string, messages []Message) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if elem, ok := c.items[key]; ok {
		entry := elem.Value.(*promptCacheEntry)
		entry.messages = append([]Message(nil), messages...)
		entry.expiresAt = c.expiresAt()
		c.lru.MoveToFront(elem)
		return
	}

	entry := &promptCacheEntry{
		key:       key,
		messages:  append([]Message(nil), messages...),
		expiresAt: c.expiresAt(),
	}
	elem := c.lru.PushFront(entry)
	c.items[key] = elem
	for len(c.items) > c.cfg.MaxEntries {
		c.removeElement(c.lru.Back())
	}
}

func (c *inMemoryPromptCache) expiresAt() time.Time {
	if c.cfg.TTL <= 0 {
		return time.Time{}
	}
	return c.now().Add(c.cfg.TTL)
}

func (c *inMemoryPromptCache) expired(entry *promptCacheEntry) bool {
	return !entry.expiresAt.IsZero() && !entry.expiresAt.After(c.now())
}

func (c *inMemoryPromptCache) removeElement(elem *list.Element) {
	if elem == nil {
		return
	}
	c.lru.Remove(elem)
	delete(c.items, elem.Value.(*promptCacheEntry).key)
}

func (c *redisPromptCache) Get(ctx context.Context, key string) ([]Message, bool) {
	data, err := c.client.Get(ctx, c.redisKey(key)).Bytes()
	if err != nil {
		return nil, false
	}
	var msgs []Message
	if err := json.Unmarshal(data, &msgs); err != nil {
		return nil, false
	}
	return msgs, true
}

func (c *redisPromptCache) Set(ctx context.Context, key string, messages []Message) {
	data, err := json.Marshal(messages)
	if err != nil {
		return
	}
	_ = c.client.Set(ctx, c.redisKey(key), data, c.cfg.TTL).Err()
}

func (c *redisPromptCache) redisKey(key string) string {
	return fmt.Sprintf("%s:%s", c.cfg.KeyPrefix, key)
}

func (c *twoLevelPromptCache) Get(ctx context.Context, key string) ([]Message, bool) {
	if c.local != nil {
		if msgs, ok := c.local.Get(ctx, key); ok {
			return msgs, true
		}
	}
	if c.remote != nil {
		if msgs, ok := c.remote.Get(ctx, key); ok {
			if c.local != nil {
				c.local.Set(ctx, key, msgs)
			}
			return msgs, true
		}
	}
	return nil, false
}

func (c *twoLevelPromptCache) Set(ctx context.Context, key string, messages []Message) {
	if c.local != nil {
		c.local.Set(ctx, key, messages)
	}
	if c.remote != nil {
		c.remote.Set(ctx, key, messages)
	}
}

var _ graph.PromptCache = rootToGraphPromptCache{}

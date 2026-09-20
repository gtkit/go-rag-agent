package ragagent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// HistoryTurn 是外部历史存储中的一轮对话。
type HistoryTurn struct {
	User      string `json:"user"`
	Assistant string `json:"assistant"`
}

// HistoryStore 是 Session 短期历史的外部存储契约，供多实例部署共享会话历史。
// Load 按写入顺序返回该 Session 的全部轮次，SDK 只取最近 MaxHistoryRounds 轮参与 prompt。
type HistoryStore interface {
	Load(ctx context.Context, sessionID string) ([]HistoryTurn, error)
	Append(ctx context.Context, sessionID string, turn HistoryTurn) error
	Clear(ctx context.Context, sessionID string) error
}

// RedisHistoryStoreConfig 配置基于 Redis List 的历史存储。
type RedisHistoryStoreConfig struct {
	KeyPrefix string
	// TTL 为每次写入后刷新的键过期时间；<= 0 表示不过期。
	TTL time.Duration
	// MaxRounds 为存储侧保留的最大轮次；<= 0 表示不截断。
	MaxRounds int
}

func (c RedisHistoryStoreConfig) normalized() RedisHistoryStoreConfig {
	c.KeyPrefix = strings.TrimSpace(c.KeyPrefix)
	if c.KeyPrefix == "" {
		c.KeyPrefix = "ragagent:history"
	}
	return c
}

type redisHistoryStore struct {
	client redis.Cmdable
	cfg    RedisHistoryStoreConfig
}

// NewRedisHistoryStore 创建 Redis 历史存储；每个 Session 对应键 <KeyPrefix>:<sessionID> 的 List，每轮一条 JSON。
func NewRedisHistoryStore(client redis.Cmdable, cfg RedisHistoryStoreConfig) (HistoryStore, error) {
	if client == nil {
		return nil, fmt.Errorf("redis history store client is required: %w", ErrInvalidConfig)
	}
	if cfg.TTL < 0 {
		return nil, fmt.Errorf("redis history store ttl must be non-negative: %w", ErrInvalidConfig)
	}
	if cfg.MaxRounds < 0 {
		return nil, fmt.Errorf("redis history store max rounds must be non-negative: %w", ErrInvalidConfig)
	}
	return &redisHistoryStore{client: client, cfg: cfg.normalized()}, nil
}

func (s *redisHistoryStore) key(sessionID string) string {
	return s.cfg.KeyPrefix + ":" + sessionID
}

func (s *redisHistoryStore) Load(ctx context.Context, sessionID string) ([]HistoryTurn, error) {
	values, err := s.client.LRange(ctx, s.key(sessionID), 0, -1).Result()
	if err != nil {
		return nil, fmt.Errorf("redis history lrange: %w", err)
	}
	turns := make([]HistoryTurn, 0, len(values))
	for _, value := range values {
		var turn HistoryTurn
		if err := json.Unmarshal([]byte(value), &turn); err != nil {
			return nil, fmt.Errorf("redis history decode turn: %w", err)
		}
		turns = append(turns, turn)
	}
	return turns, nil
}

func (s *redisHistoryStore) Append(ctx context.Context, sessionID string, turn HistoryTurn) error {
	data, err := json.Marshal(turn)
	if err != nil {
		return fmt.Errorf("redis history encode turn: %w", err)
	}
	key := s.key(sessionID)
	pipe := s.client.TxPipeline()
	pipe.RPush(ctx, key, data)
	if s.cfg.MaxRounds > 0 {
		pipe.LTrim(ctx, key, int64(-s.cfg.MaxRounds), -1)
	}
	if s.cfg.TTL > 0 {
		pipe.Expire(ctx, key, s.cfg.TTL)
	}
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("redis history append: %w", err)
	}
	return nil
}

func (s *redisHistoryStore) Clear(ctx context.Context, sessionID string) error {
	if err := s.client.Del(ctx, s.key(sessionID)).Err(); err != nil {
		return fmt.Errorf("redis history clear: %w", err)
	}
	return nil
}

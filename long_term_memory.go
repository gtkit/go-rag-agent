package ragagent

import (
	"context"
	"fmt"
	"math"
	"slices"
	"sync"
	"time"
)

// MemoryComponents 定义长期记忆注入组件。
type MemoryComponents struct {
	LongTermMemory LongTermMemoryStore
}

// LongTermMemoryRecord 表示一条长期记忆记录。
type LongTermMemoryRecord struct {
	ID        string
	SessionID string
	User      string
	Assistant string
	Embedding []float32
	CreatedAt time.Time
}

// LongTermMemoryHit 表示一次长期记忆检索结果。
type LongTermMemoryHit struct {
	Memory LongTermMemoryRecord
	Score  float32
}

// LongTermMemoryStore 定义长期记忆存储能力。
type LongTermMemoryStore interface {
	Store(ctx context.Context, records []LongTermMemoryRecord) error
	Search(ctx context.Context, sessionID string, queryEmbedding []float32, topK int, threshold float32) ([]LongTermMemoryHit, error)
	ClearSession(ctx context.Context, sessionID string) error
	Close() error
}

type inMemoryLongTermMemoryStore struct {
	mu      sync.RWMutex
	records []LongTermMemoryRecord
}

// NewInMemoryLongTermMemoryStore 创建默认 in-memory 长期记忆实现。
func NewInMemoryLongTermMemoryStore() LongTermMemoryStore {
	return &inMemoryLongTermMemoryStore{
		records: make([]LongTermMemoryRecord, 0),
	}
}

func (s *inMemoryLongTermMemoryStore) Store(ctx context.Context, records []LongTermMemoryRecord) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(records) == 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, record := range records {
		if record.ID == "" {
			return fmt.Errorf("memory id is required")
		}
		if record.SessionID == "" {
			return fmt.Errorf("memory session id is required")
		}
		if len(record.Embedding) == 0 {
			return fmt.Errorf("memory embedding is required")
		}
		s.records = append(s.records, LongTermMemoryRecord{
			ID:        record.ID,
			SessionID: record.SessionID,
			User:      record.User,
			Assistant: record.Assistant,
			Embedding: slices.Clone(record.Embedding),
			CreatedAt: record.CreatedAt,
		})
	}
	return nil
}

func (s *inMemoryLongTermMemoryStore) Search(ctx context.Context, sessionID string, queryEmbedding []float32, topK int, threshold float32) ([]LongTermMemoryHit, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if topK <= 0 || len(queryEmbedding) == 0 {
		return nil, nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	hits := make([]LongTermMemoryHit, 0, len(s.records))
	for _, record := range s.records {
		if record.SessionID != sessionID {
			continue
		}
		score, ok := cosineSimilarity(queryEmbedding, record.Embedding)
		if !ok || score < threshold {
			continue
		}
		hits = append(hits, LongTermMemoryHit{
			Memory: LongTermMemoryRecord{
				ID:        record.ID,
				SessionID: record.SessionID,
				User:      record.User,
				Assistant: record.Assistant,
				Embedding: slices.Clone(record.Embedding),
				CreatedAt: record.CreatedAt,
			},
			Score: score,
		})
	}
	slices.SortFunc(hits, func(a, b LongTermMemoryHit) int {
		switch {
		case a.Score > b.Score:
			return -1
		case a.Score < b.Score:
			return 1
		default:
			switch {
			case a.Memory.ID < b.Memory.ID:
				return -1
			case a.Memory.ID > b.Memory.ID:
				return 1
			default:
				return 0
			}
		}
	})
	if len(hits) > topK {
		hits = hits[:topK]
	}
	return hits, nil
}

func (s *inMemoryLongTermMemoryStore) ClearSession(ctx context.Context, sessionID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	filtered := s.records[:0]
	for _, record := range s.records {
		if record.SessionID == sessionID {
			continue
		}
		filtered = append(filtered, record)
	}
	s.records = filtered
	return nil
}

func (s *inMemoryLongTermMemoryStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records = nil
	return nil
}

func cosineSimilarity(a []float32, b []float32) (float32, bool) {
	if len(a) == 0 || len(a) != len(b) {
		return 0, false
	}
	var dot float64
	var normA float64
	var normB float64
	for i := range a {
		dot += float64(a[i] * b[i])
		normA += float64(a[i] * a[i])
		normB += float64(b[i] * b[i])
	}
	if normA == 0 || normB == 0 {
		return 0, false
	}
	return float32(dot / (math.Sqrt(normA) * math.Sqrt(normB))), true
}

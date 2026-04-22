package ragagent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"slices"
	"strings"
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
	UserID    string
	Tenant    string
	User      string
	Assistant string
	Embedding []float32
	CreatedAt time.Time
	ExpiresAt time.Time
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

// LongTermMemoryMaintenance 定义长期记忆的可选维护接口。
type LongTermMemoryMaintenance interface {
	PruneExpired(ctx context.Context, now time.Time) (int, error)
}

type inMemoryLongTermMemoryStore struct {
	mu      sync.RWMutex
	records []LongTermMemoryRecord
}

const (
	longTermMemorySourcePrefix      = "/ragagent/memory/"
	longTermMemoryMetadataUserIDKey = "rag_memory_user_id"
	longTermMemoryMetadataTenantKey = "rag_memory_tenant"
	longTermMemoryMetadataUserKey   = "rag_memory_user"
	longTermMemoryMetadataReplyKey  = "rag_memory_assistant"
	longTermMemoryMetadataTimeKey   = "rag_memory_created_at"
	longTermMemoryMetadataExpireKey = "rag_memory_expires_at"
)

// NewInMemoryLongTermMemoryStore 创建默认 in-memory 长期记忆实现。
func NewInMemoryLongTermMemoryStore() LongTermMemoryStore {
	return &inMemoryLongTermMemoryStore{
		records: make([]LongTermMemoryRecord, 0),
	}
}

type vectorLongTermMemoryStore struct {
	store VectorStore
}

// NewVectorLongTermMemoryStore 创建一个基于根包 VectorStore 的长期记忆实现。
func NewVectorLongTermMemoryStore(store VectorStore) LongTermMemoryStore {
	return &vectorLongTermMemoryStore{store: store}
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
		normalized := LongTermMemoryRecord{
			ID:        record.ID,
			SessionID: record.SessionID,
			UserID:    record.UserID,
			Tenant:    record.Tenant,
			User:      record.User,
			Assistant: record.Assistant,
			Embedding: slices.Clone(record.Embedding),
			CreatedAt: record.CreatedAt,
			ExpiresAt: record.ExpiresAt,
		}
		replaced := false
		for i := range s.records {
			if s.records[i].ID != normalized.ID {
				continue
			}
			s.records[i] = normalized
			replaced = true
			break
		}
		if !replaced {
			s.records = append(s.records, normalized)
		}
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
		if isLongTermMemoryExpired(record, time.Now()) {
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
				UserID:    record.UserID,
				Tenant:    record.Tenant,
				User:      record.User,
				Assistant: record.Assistant,
				Embedding: slices.Clone(record.Embedding),
				CreatedAt: record.CreatedAt,
				ExpiresAt: record.ExpiresAt,
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

func (s *inMemoryLongTermMemoryStore) PruneExpired(ctx context.Context, now time.Time) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	filtered := s.records[:0]
	pruned := 0
	for _, record := range s.records {
		if isLongTermMemoryExpired(record, now) {
			pruned++
			continue
		}
		filtered = append(filtered, record)
	}
	s.records = filtered
	return pruned, nil
}

func (s *vectorLongTermMemoryStore) Store(ctx context.Context, records []LongTermMemoryRecord) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if s == nil || s.store == nil {
		return fmt.Errorf("vector store is required")
	}
	if len(records) == 0 {
		return nil
	}

	chunks := make([]ChunkRecord, 0, len(records))
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
		chunks = append(chunks, ChunkRecord{
			ChunkID:    record.ID,
			ParentID:   record.ID,
			SourcePath: longTermMemorySourcePath(record.SessionID),
			Title:      "long_term_memory",
			Text:       strings.TrimSpace(record.User) + "\n" + strings.TrimSpace(record.Assistant),
			StartRune:  0,
			EndRune:    len([]rune(strings.TrimSpace(record.User) + "\n" + strings.TrimSpace(record.Assistant))),
			Metadata: map[string]string{
				longTermMemoryMetadataUserIDKey: record.UserID,
				longTermMemoryMetadataTenantKey: record.Tenant,
				longTermMemoryMetadataUserKey:   record.User,
				longTermMemoryMetadataReplyKey:  record.Assistant,
				longTermMemoryMetadataTimeKey:   record.CreatedAt.Format(time.RFC3339Nano),
				longTermMemoryMetadataExpireKey: formatLongTermMemoryTimestamp(record.ExpiresAt),
			},
			Embedding: slices.Clone(record.Embedding),
		})
	}
	return s.store.Upsert(ctx, chunks)
}

func (s *vectorLongTermMemoryStore) Search(ctx context.Context, sessionID string, queryEmbedding []float32, topK int, threshold float32) ([]LongTermMemoryHit, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if s == nil || s.store == nil {
		return nil, fmt.Errorf("vector store is required")
	}
	if topK <= 0 || len(queryEmbedding) == 0 || strings.TrimSpace(sessionID) == "" {
		return nil, nil
	}

	hits, err := s.store.SearchWithFilter(ctx, queryEmbedding, topK, threshold, SearchFilter{
		SourcePaths: []string{longTermMemorySourcePath(sessionID)},
	})
	if err != nil {
		return nil, err
	}

	records := make([]LongTermMemoryHit, 0, len(hits))
	for _, hit := range hits {
		record := LongTermMemoryRecord{
			ID:        hit.Chunk.ChunkID,
			SessionID: sessionID,
			UserID:    hit.Chunk.Metadata[longTermMemoryMetadataUserIDKey],
			Tenant:    hit.Chunk.Metadata[longTermMemoryMetadataTenantKey],
			User:      hit.Chunk.Metadata[longTermMemoryMetadataUserKey],
			Assistant: hit.Chunk.Metadata[longTermMemoryMetadataReplyKey],
			Embedding: slices.Clone(hit.Chunk.Embedding),
		}
		if ts := hit.Chunk.Metadata[longTermMemoryMetadataTimeKey]; ts != "" {
			createdAt, parseErr := time.Parse(time.RFC3339Nano, ts)
			if parseErr == nil {
				record.CreatedAt = createdAt
			}
		}
		if ts := hit.Chunk.Metadata[longTermMemoryMetadataExpireKey]; ts != "" {
			expiresAt, parseErr := time.Parse(time.RFC3339Nano, ts)
			if parseErr == nil {
				record.ExpiresAt = expiresAt
			}
		}
		if isLongTermMemoryExpired(record, time.Now()) {
			continue
		}
		records = append(records, LongTermMemoryHit{
			Memory: record,
			Score:  hit.Score,
		})
	}
	return records, nil
}

func (s *vectorLongTermMemoryStore) ClearSession(ctx context.Context, sessionID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if s == nil || s.store == nil {
		return fmt.Errorf("vector store is required")
	}
	if strings.TrimSpace(sessionID) == "" {
		return nil
	}
	return s.store.DeleteBySourcePaths(ctx, []string{longTermMemorySourcePath(sessionID)})
}

func (s *vectorLongTermMemoryStore) Close() error {
	if s == nil || s.store == nil {
		return nil
	}
	return s.store.Close()
}

func longTermMemorySourcePath(sessionID string) string {
	return longTermMemorySourcePrefix + strings.TrimSpace(sessionID)
}

func isLongTermMemoryExpired(record LongTermMemoryRecord, now time.Time) bool {
	return !record.ExpiresAt.IsZero() && !record.ExpiresAt.After(now)
}

func formatLongTermMemoryTimestamp(ts time.Time) string {
	if ts.IsZero() {
		return ""
	}
	return ts.Format(time.RFC3339Nano)
}

func compressLongTermMemoryTurn(user string, assistant string, maxRunes int) (string, string) {
	user = strings.Join(strings.Fields(strings.TrimSpace(user)), " ")
	assistant = strings.Join(strings.Fields(strings.TrimSpace(assistant)), " ")
	if maxRunes <= 0 {
		return user, assistant
	}
	if len([]rune(user))+len([]rune(assistant)) <= maxRunes {
		return user, assistant
	}
	budgetUser := maxRunes / 2
	budgetAssistant := maxRunes - budgetUser
	return trimRunes(user, budgetUser), trimRunes(assistant, budgetAssistant)
}

func trimRunes(input string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	runes := []rune(input)
	if len(runes) <= maxRunes {
		return input
	}
	if maxRunes <= 3 {
		return string(runes[:maxRunes])
	}
	return string(runes[:maxRunes-3]) + "..."
}

func buildLongTermMemoryID(sessionID string, scope MemoryScope, user string, assistant string) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{
		strings.TrimSpace(scope.Tenant),
		strings.TrimSpace(scope.UserID),
		strings.TrimSpace(sessionID),
		strings.Join(strings.Fields(strings.TrimSpace(user)), " "),
		strings.Join(strings.Fields(strings.TrimSpace(assistant)), " "),
	}, "\n")))
	return "mem_" + hex.EncodeToString(sum[:16])
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

package storage

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"

	chromem "github.com/philippgille/chromem-go"
)

const (
	defaultCollectionName = "knowledge"

	metadataKeySourcePath = "rag_source_path"
	metadataKeyTitle      = "rag_title"
	metadataKeyParentID   = "rag_parent_id"
	metadataKeyStartRune  = "rag_start_rune"
	metadataKeyEndRune    = "rag_end_rune"
	metadataKeyUserPrefix = "rag_meta_"
)

// ChromemStore is a chromem-backed VectorStore.
type ChromemStore struct {
	db         *chromem.DB
	collection *chromem.Collection
	upsertMu   sync.Mutex
	// afterAddHook is test-only and runs after add/overwrite, before stale cleanup.
	afterAddHook func()
}

func NewChromemStore(cfg Config) (*ChromemStore, error) {
	collectionName := cfg.Collection
	if collectionName == "" {
		collectionName = defaultCollectionName
	}

	var (
		db  *chromem.DB
		err error
	)
	if cfg.DataDir == "" {
		db = chromem.NewDB()
	} else {
		persistDir := filepath.Join(cfg.DataDir, "chromem")
		db, err = chromem.NewPersistentDB(persistDir, false)
		if err != nil {
			return nil, fmt.Errorf("create persistent chromem db: %w", err)
		}
	}

	collection, err := db.GetOrCreateCollection(collectionName, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("get or create collection %q: %w", collectionName, err)
	}

	return &ChromemStore{
		db:         db,
		collection: collection,
	}, nil
}

func (s *ChromemStore) Upsert(ctx context.Context, chunks []ChunkRecord) error {
	s.upsertMu.Lock()
	defer s.upsertMu.Unlock()

	if len(chunks) == 0 {
		return nil
	}

	seenChunkIDs := make(map[string]struct{}, len(chunks))
	parentIDSet := make(map[string]struct{}, len(chunks))
	parentIDToChunkIDSet := make(map[string]map[string]struct{}, len(chunks))
	parentIDToQueryEmbedding := make(map[string][]float32, len(chunks))
	for _, chunk := range chunks {
		if chunk.ChunkID == "" {
			return fmt.Errorf("chunk id is empty")
		}
		if chunk.ParentID == "" {
			return fmt.Errorf("parent id is empty for chunk %q", chunk.ChunkID)
		}
		if len(chunk.Embedding) == 0 {
			return fmt.Errorf("embedding is empty for chunk %q", chunk.ChunkID)
		}
		if _, exists := seenChunkIDs[chunk.ChunkID]; exists {
			return fmt.Errorf("duplicate chunk id %q in upsert batch", chunk.ChunkID)
		}
		seenChunkIDs[chunk.ChunkID] = struct{}{}
		parentIDSet[chunk.ParentID] = struct{}{}
		if _, ok := parentIDToChunkIDSet[chunk.ParentID]; !ok {
			parentIDToChunkIDSet[chunk.ParentID] = make(map[string]struct{})
		}
		parentIDToChunkIDSet[chunk.ParentID][chunk.ChunkID] = struct{}{}
		if _, ok := parentIDToQueryEmbedding[chunk.ParentID]; !ok {
			parentIDToQueryEmbedding[chunk.ParentID] = slices.Clone(chunk.Embedding)
		}
	}

	if err := ctx.Err(); err != nil {
		return fmt.Errorf("upsert chunks: %w", err)
	}
	internalCtx := context.WithoutCancel(ctx)

	parentIDs := make([]string, 0, len(parentIDSet))
	for parentID := range parentIDSet {
		parentIDs = append(parentIDs, parentID)
	}
	sort.Strings(parentIDs)

	docs := make([]chromem.Document, 0, len(chunks))
	for _, chunk := range chunks {
		doc := chromem.Document{
			ID:        chunk.ChunkID,
			Metadata:  chunkToMetadata(chunk),
			Embedding: slices.Clone(chunk.Embedding),
			Content:   chunk.Text,
		}
		docs = append(docs, doc)
	}

	if err := s.collection.AddDocuments(internalCtx, docs, 1); err != nil {
		return fmt.Errorf("upsert chunks: %w", err)
	}
	if s.afterAddHook != nil {
		s.afterAddHook()
	}

	for _, parentID := range parentIDs {
		currentIDs, err := s.listChunkIDsByParent(internalCtx, parentID, parentIDToQueryEmbedding[parentID])
		if err != nil {
			return fmt.Errorf("list chunk ids by parent %q: %w", parentID, err)
		}

		staleIDs := make([]string, 0, len(currentIDs))
		currentSet := parentIDToChunkIDSet[parentID]
		for _, currentID := range currentIDs {
			if _, ok := currentSet[currentID]; !ok {
				staleIDs = append(staleIDs, currentID)
			}
		}
		if len(staleIDs) == 0 {
			continue
		}
		if err := s.collection.Delete(internalCtx, nil, nil, staleIDs...); err != nil {
			return fmt.Errorf("delete stale chunks for parent %q: %w", parentID, err)
		}
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("upsert chunks: %w", err)
	}

	return nil
}

func (s *ChromemStore) Search(ctx context.Context, queryEmbedding []float32, topK int, threshold float32) ([]SearchHit, error) {
	if len(queryEmbedding) == 0 {
		return nil, fmt.Errorf("query embedding is empty")
	}
	if topK <= 0 {
		return nil, fmt.Errorf("topK must be > 0")
	}

	count := s.collection.Count()
	if count == 0 {
		return nil, nil
	}
	if topK > count {
		topK = count
	}

	results, err := s.collection.QueryEmbedding(ctx, queryEmbedding, topK, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("query embedding: %w", err)
	}

	hits := make([]SearchHit, 0, len(results))
	for _, result := range results {
		if result.Similarity < threshold {
			continue
		}
		hits = append(hits, SearchHit{
			Chunk: resultToChunk(result),
			Score: result.Similarity,
		})
	}

	return hits, nil
}

func (s *ChromemStore) Close() error {
	_ = s.db
	return nil
}

func chunkToMetadata(chunk ChunkRecord) map[string]string {
	metadata := make(map[string]string, len(chunk.Metadata)+5)
	metadata[metadataKeySourcePath] = chunk.SourcePath
	metadata[metadataKeyTitle] = chunk.Title
	metadata[metadataKeyParentID] = chunk.ParentID
	metadata[metadataKeyStartRune] = strconv.Itoa(chunk.StartRune)
	metadata[metadataKeyEndRune] = strconv.Itoa(chunk.EndRune)
	for key, value := range chunk.Metadata {
		metadata[metadataKeyUserPrefix+key] = value
	}
	return metadata
}

func resultToChunk(result chromem.Result) ChunkRecord {
	return ChunkRecord{
		ChunkID:    result.ID,
		ParentID:   result.Metadata[metadataKeyParentID],
		SourcePath: result.Metadata[metadataKeySourcePath],
		Title:      result.Metadata[metadataKeyTitle],
		Text:       result.Content,
		StartRune:  parseIntMetadata(result.Metadata, metadataKeyStartRune),
		EndRune:    parseIntMetadata(result.Metadata, metadataKeyEndRune),
		Metadata:   extractUserMetadata(result.Metadata),
		Embedding:  slices.Clone(result.Embedding),
	}
}

func extractUserMetadata(metadata map[string]string) map[string]string {
	if len(metadata) == 0 {
		return nil
	}

	extra := make(map[string]string, len(metadata))
	for key, value := range metadata {
		if strings.HasPrefix(key, metadataKeyUserPrefix) {
			extra[strings.TrimPrefix(key, metadataKeyUserPrefix)] = value
		}
	}
	if len(extra) == 0 {
		return nil
	}
	return extra
}

func parseIntMetadata(metadata map[string]string, key string) int {
	raw, ok := metadata[key]
	if !ok || raw == "" {
		return 0
	}

	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0
	}
	return value
}

func (s *ChromemStore) listChunkIDsByParent(ctx context.Context, parentID string, queryEmbedding []float32) ([]string, error) {
	count := s.collection.Count()
	if count == 0 {
		return nil, nil
	}

	results, err := s.collection.QueryEmbedding(ctx, queryEmbedding, count, map[string]string{metadataKeyParentID: parentID}, nil)
	if err != nil {
		return nil, fmt.Errorf("query parent chunks: %w", err)
	}

	ids := make([]string, 0, len(results))
	for _, result := range results {
		ids = append(ids, result.ID)
	}
	return ids, nil
}

package ragagent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/gtkit/pgorm"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	pgvector "github.com/pgvector/pgvector-go"
)

const (
	pgVectorDistanceCosine     = "cosine"
	pgVectorIndexNone          = "none"
	pgVectorIndexHNSW          = "hnsw"
	pgVectorIndexIVFFlat       = "ivfflat"
	defaultPGVectorSchemaName  = "public"
	defaultPGVectorHNSWM       = 16
	defaultPGVectorEfConstruct = 64
	defaultPGVectorEfSearch    = 100
	defaultPGVectorLists       = 100
	defaultPGVectorProbes      = 10
	defaultPGVectorUpsertBatch = 200
)

// PGVectorStoreConfig 定义 PostgreSQL/pgvector store 的构造配置。
type PGVectorStoreConfig struct {
	Pool                *pgxpool.Pool
	PGORMClient         *pgorm.Client
	PGORMConfig         *pgorm.Config
	ConnString          string
	SchemaName          string
	TableName           string
	Dimensions          int
	DistanceMetric      string
	AutoCreateExtension bool
	AutoCreateSchema    bool
	AutoCreateTable     bool
	AutoCreateIndexes   bool
	IndexStrategy       string
	UpsertBatchSize     int
	HNSWM               int
	HNSWEfConstruction  int
	HNSWEfSearch        int
	IVFFlatLists        int
	IVFFlatProbes       int
}

func (c PGVectorStoreConfig) normalized() PGVectorStoreConfig {
	c.ConnString = strings.TrimSpace(c.ConnString)
	c.SchemaName = strings.TrimSpace(c.SchemaName)
	c.TableName = strings.TrimSpace(c.TableName)
	c.DistanceMetric = strings.ToLower(strings.TrimSpace(c.DistanceMetric))
	c.IndexStrategy = strings.ToLower(strings.TrimSpace(c.IndexStrategy))

	if c.SchemaName == "" {
		c.SchemaName = defaultPGVectorSchemaName
	}
	if c.DistanceMetric == "" {
		c.DistanceMetric = pgVectorDistanceCosine
	}
	if c.IndexStrategy == "" {
		c.IndexStrategy = pgVectorIndexNone
	}
	if c.UpsertBatchSize == 0 {
		c.UpsertBatchSize = defaultPGVectorUpsertBatch
	}
	if !c.AutoCreateExtension {
		c.AutoCreateExtension = true
	}
	if !c.AutoCreateSchema {
		c.AutoCreateSchema = true
	}
	if !c.AutoCreateTable {
		c.AutoCreateTable = true
	}
	if !c.AutoCreateIndexes {
		c.AutoCreateIndexes = true
	}
	if c.HNSWM == 0 {
		c.HNSWM = defaultPGVectorHNSWM
	}
	if c.HNSWEfConstruction == 0 {
		c.HNSWEfConstruction = defaultPGVectorEfConstruct
	}
	if c.HNSWEfSearch == 0 {
		c.HNSWEfSearch = defaultPGVectorEfSearch
	}
	if c.IVFFlatLists == 0 {
		c.IVFFlatLists = defaultPGVectorLists
	}
	if c.IVFFlatProbes == 0 {
		c.IVFFlatProbes = defaultPGVectorProbes
	}
	return c
}

func (c PGVectorStoreConfig) validate() error {
	c = c.normalized()

	if count := c.connectionSourceCount(); count == 0 {
		return fmt.Errorf("pgvector connection source is required: %w", ErrInvalidConfig)
	} else if count > 1 {
		return fmt.Errorf("pgvector connection sources must be mutually exclusive: %w", ErrInvalidConfig)
	}
	if c.TableName == "" {
		return fmt.Errorf("pgvector table name is required: %w", ErrInvalidConfig)
	}
	if c.Dimensions <= 0 {
		return fmt.Errorf("pgvector dimensions must be positive: %w", ErrInvalidConfig)
	}
	switch c.DistanceMetric {
	case pgVectorDistanceCosine:
	default:
		return fmt.Errorf("pgvector distance metric %q is unsupported: %w", c.DistanceMetric, ErrInvalidConfig)
	}
	switch c.IndexStrategy {
	case pgVectorIndexNone, pgVectorIndexHNSW, pgVectorIndexIVFFlat:
	default:
		return fmt.Errorf("pgvector index strategy %q is unsupported: %w", c.IndexStrategy, ErrInvalidConfig)
	}
	if c.UpsertBatchSize <= 0 {
		return fmt.Errorf("pgvector upsert batch size must be positive: %w", ErrInvalidConfig)
	}
	return nil
}

func (c PGVectorStoreConfig) connectionSourceCount() int {
	count := 0
	if c.Pool != nil {
		count++
	}
	if c.PGORMClient != nil {
		count++
	}
	if c.PGORMConfig != nil {
		count++
	}
	if c.ConnString != "" {
		count++
	}
	return count
}

type pgVectorStore struct {
	pool      *pgxpool.Pool
	ownsPool  bool
	pgorm     *pgorm.Client
	cfg       PGVectorStoreConfig
	tableName string
}

// NewPGVectorStore 创建 PostgreSQL/pgvector store。
func NewPGVectorStore(cfg PGVectorStoreConfig) (VectorStore, error) {
	cfg = cfg.normalized()
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	pool := cfg.Pool
	ownsPool := false
	var pgormClient *pgorm.Client
	switch {
	case cfg.PGORMClient != nil:
		pgormClient = cfg.PGORMClient
		pool = pgormClient.Pool()
		if pool == nil {
			return nil, fmt.Errorf("pgorm client pool is nil: %w", ErrInvalidConfig)
		}
	case cfg.PGORMConfig != nil:
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		client, err := cfg.PGORMConfig.Clone().Open(ctx)
		if err != nil {
			return nil, fmt.Errorf("open pgorm client: %w", err)
		}
		pgormClient = client
		pool = client.Pool()
		ownsPool = false
	case pool == nil:
		parsed, err := pgxpool.ParseConfig(cfg.ConnString)
		if err != nil {
			return nil, fmt.Errorf("parse pgvector conn string: %w", err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		pool, err = pgxpool.NewWithConfig(ctx, parsed)
		if err != nil {
			return nil, fmt.Errorf("create pgvector pool: %w", err)
		}
		ownsPool = true
	}

	store := &pgVectorStore{
		pool:      pool,
		ownsPool:  ownsPool,
		pgorm:     pgormClient,
		cfg:       cfg,
		tableName: pgx.Identifier{cfg.SchemaName, cfg.TableName}.Sanitize(),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := store.init(ctx); err != nil {
		if ownsPool {
			pool.Close()
		}
		return nil, err
	}
	return store, nil
}

func (s *pgVectorStore) init(ctx context.Context) error {
	if s.cfg.AutoCreateExtension {
		if _, err := s.pool.Exec(ctx, `CREATE EXTENSION IF NOT EXISTS vector`); err != nil && !isDuplicateExtensionRace(err) {
			return fmt.Errorf("create vector extension: %w", err)
		}
	}
	if s.cfg.AutoCreateSchema && s.cfg.SchemaName != "" && s.cfg.SchemaName != defaultPGVectorSchemaName {
		if _, err := s.pool.Exec(ctx, fmt.Sprintf(`CREATE SCHEMA IF NOT EXISTS %s`, pgx.Identifier{s.cfg.SchemaName}.Sanitize())); err != nil {
			return fmt.Errorf("create schema: %w", err)
		}
	}
	if s.cfg.AutoCreateTable {
		if _, err := s.pool.Exec(ctx, s.createTableSQL()); err != nil {
			return fmt.Errorf("create table: %w", err)
		}
	}
	if s.cfg.AutoCreateIndexes {
		for _, stmt := range s.indexStatements() {
			if _, err := s.pool.Exec(ctx, stmt); err != nil {
				return fmt.Errorf("create index: %w", err)
			}
		}
	}
	return nil
}

func isDuplicateExtensionRace(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		if pgErr.Code == "23505" && strings.Contains(pgErr.Message, "pg_extension_name_index") {
			return true
		}
	}
	return false
}

func (s *pgVectorStore) createTableSQL() string {
	return fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS %s (
    chunk_id text PRIMARY KEY,
    parent_id text NOT NULL,
    source_path text NOT NULL,
    title text NOT NULL,
    content text NOT NULL,
    start_rune integer NOT NULL,
    end_rune integer NOT NULL,
    metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
    embedding vector(%d) NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
)`, s.tableName, s.cfg.Dimensions)
}

func (s *pgVectorStore) indexStatements() []string {
	stmts := []string{
		fmt.Sprintf(`CREATE INDEX IF NOT EXISTS %s ON %s (parent_id)`, pgx.Identifier{s.cfg.TableName + "_parent_id_idx"}.Sanitize(), s.tableName),
		fmt.Sprintf(`CREATE INDEX IF NOT EXISTS %s ON %s (source_path)`, pgx.Identifier{s.cfg.TableName + "_source_path_idx"}.Sanitize(), s.tableName),
		fmt.Sprintf(`CREATE INDEX IF NOT EXISTS %s ON %s (source_path text_pattern_ops)`, pgx.Identifier{s.cfg.TableName + "_source_path_prefix_idx"}.Sanitize(), s.tableName),
		fmt.Sprintf(`CREATE INDEX IF NOT EXISTS %s ON %s USING gin (metadata jsonb_path_ops)`, pgx.Identifier{s.cfg.TableName + "_metadata_gin_idx"}.Sanitize(), s.tableName),
	}
	switch s.cfg.IndexStrategy {
	case pgVectorIndexHNSW:
		stmts = append(stmts, fmt.Sprintf(
			`CREATE INDEX IF NOT EXISTS %s ON %s USING hnsw (embedding vector_cosine_ops) WITH (m = %d, ef_construction = %d)`,
			pgx.Identifier{s.cfg.TableName + "_embedding_hnsw_cosine_idx"}.Sanitize(),
			s.tableName,
			s.cfg.HNSWM,
			s.cfg.HNSWEfConstruction,
		))
	case pgVectorIndexIVFFlat:
		stmts = append(stmts, fmt.Sprintf(
			`CREATE INDEX IF NOT EXISTS %s ON %s USING ivfflat (embedding vector_cosine_ops) WITH (lists = %d)`,
			pgx.Identifier{s.cfg.TableName + "_embedding_ivfflat_cosine_idx"}.Sanitize(),
			s.tableName,
			s.cfg.IVFFlatLists,
		))
	}
	return stmts
}

func (s *pgVectorStore) Upsert(ctx context.Context, chunks []ChunkRecord) error {
	if len(chunks) == 0 {
		return nil
	}
	parentToChunkIDs := make(map[string][]string)
	for _, chunk := range chunks {
		if err := s.validateChunk(chunk); err != nil {
			return err
		}
		parentToChunkIDs[chunk.ParentID] = append(parentToChunkIDs[chunk.ParentID], chunk.ChunkID)
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin pgvector upsert tx: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	const upsertSQL = `
INSERT INTO %s (
    chunk_id,
    parent_id,
    source_path,
    title,
    content,
    start_rune,
    end_rune,
    metadata,
    embedding,
    updated_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8::jsonb, $9::vector, now()
)
ON CONFLICT (chunk_id) DO UPDATE SET
    parent_id = EXCLUDED.parent_id,
    source_path = EXCLUDED.source_path,
    title = EXCLUDED.title,
    content = EXCLUDED.content,
    start_rune = EXCLUDED.start_rune,
    end_rune = EXCLUDED.end_rune,
    metadata = EXCLUDED.metadata,
    embedding = EXCLUDED.embedding,
    updated_at = now()`

	for start := 0; start < len(chunks); start += s.cfg.UpsertBatchSize {
		end := min(start+s.cfg.UpsertBatchSize, len(chunks))
		var batch pgx.Batch
		for _, chunk := range chunks[start:end] {
			metadataJSON, err := marshalMetadataJSON(chunk.Metadata)
			if err != nil {
				return fmt.Errorf("marshal chunk metadata: %w", err)
			}
			batch.Queue(fmt.Sprintf(upsertSQL, s.tableName),
				chunk.ChunkID,
				chunk.ParentID,
				chunk.SourcePath,
				chunk.Title,
				chunk.Text,
				chunk.StartRune,
				chunk.EndRune,
				metadataJSON,
				pgvector.NewVector(chunk.Embedding).String(),
			)
		}
		results := tx.SendBatch(ctx, &batch)
		for _, chunk := range chunks[start:end] {
			if _, err := results.Exec(); err != nil {
				_ = results.Close()
				return fmt.Errorf("upsert pgvector chunk %q: %w", chunk.ChunkID, err)
			}
		}
		if err := results.Close(); err != nil {
			return fmt.Errorf("close pgvector upsert batch: %w", err)
		}
	}

	for parentID, chunkIDs := range parentToChunkIDs {
		if _, err := tx.Exec(ctx, fmt.Sprintf(`DELETE FROM %s WHERE parent_id = $1 AND NOT (chunk_id = ANY($2::text[]))`, s.tableName), parentID, chunkIDs); err != nil {
			return fmt.Errorf("delete stale pgvector chunks for parent %q: %w", parentID, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit pgvector upsert tx: %w", err)
	}
	return nil
}

func (s *pgVectorStore) SearchWithFilter(ctx context.Context, queryEmbedding []float32, topK int, threshold float32, filter SearchFilter) ([]SearchHit, error) {
	if len(queryEmbedding) == 0 {
		return nil, fmt.Errorf("query embedding is empty")
	}
	if topK <= 0 {
		return nil, fmt.Errorf("topK must be > 0")
	}
	if len(queryEmbedding) != s.cfg.Dimensions {
		return nil, fmt.Errorf("query embedding dimensions %d do not match store dimensions %d", len(queryEmbedding), s.cfg.Dimensions)
	}

	sql, args, err := s.buildSearchSQL(queryEmbedding, topK, threshold, filter)
	if err != nil {
		return nil, err
	}

	rows, err := s.queryRows(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("search pgvector store: %w", err)
	}
	defer rows.Close()

	hits := make([]SearchHit, 0)
	for rows.Next() {
		var (
			chunk        ChunkRecord
			metadataJSON []byte
			score        float32
		)
		if err := rows.Scan(
			&chunk.ChunkID,
			&chunk.ParentID,
			&chunk.SourcePath,
			&chunk.Title,
			&chunk.Text,
			&chunk.StartRune,
			&chunk.EndRune,
			&metadataJSON,
			&score,
		); err != nil {
			return nil, fmt.Errorf("scan pgvector search row: %w", err)
		}
		if len(metadataJSON) > 0 {
			if err := json.Unmarshal(metadataJSON, &chunk.Metadata); err != nil {
				return nil, fmt.Errorf("unmarshal pgvector row metadata: %w", err)
			}
		}
		if chunk.Metadata == nil {
			chunk.Metadata = map[string]string{}
		}
		hits = append(hits, SearchHit{
			Chunk: chunk,
			Score: score,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate pgvector search rows: %w", err)
	}
	return hits, nil
}

func (s *pgVectorStore) Search(ctx context.Context, queryEmbedding []float32, topK int, threshold float32) ([]SearchHit, error) {
	return s.SearchWithFilter(ctx, queryEmbedding, topK, threshold, SearchFilter{})
}

func (s *pgVectorStore) DeleteBySourcePaths(ctx context.Context, sourcePaths []string) error {
	if len(sourcePaths) == 0 {
		return nil
	}
	if _, err := s.pool.Exec(ctx, fmt.Sprintf(`DELETE FROM %s WHERE source_path = ANY($1::text[])`, s.tableName), sourcePaths); err != nil {
		return fmt.Errorf("delete pgvector source paths: %w", err)
	}
	return nil
}

func (s *pgVectorStore) Close() error {
	if s.pgorm != nil && s.cfg.PGORMConfig != nil {
		s.pgorm.Close()
		return nil
	}
	if s.ownsPool && s.pool != nil {
		s.pool.Close()
	}
	return nil
}

func (s *pgVectorStore) validateChunk(chunk ChunkRecord) error {
	if chunk.ChunkID == "" {
		return fmt.Errorf("chunk id is empty")
	}
	if chunk.ParentID == "" {
		return fmt.Errorf("parent id is empty for chunk %q", chunk.ChunkID)
	}
	if len(chunk.Embedding) != s.cfg.Dimensions {
		return fmt.Errorf("chunk %q embedding dimensions %d do not match store dimensions %d", chunk.ChunkID, len(chunk.Embedding), s.cfg.Dimensions)
	}
	return nil
}

func marshalMetadataJSON(metadata map[string]string) ([]byte, error) {
	if len(metadata) == 0 {
		return []byte(`{}`), nil
	}
	return json.Marshal(metadata)
}

func (s *pgVectorStore) buildSearchSQL(queryEmbedding []float32, topK int, threshold float32, filter SearchFilter) (string, []any, error) {
	args := []any{pgvector.NewVector(queryEmbedding).String()}
	var builder strings.Builder
	builder.WriteString(`SELECT chunk_id, parent_id, source_path, title, content, start_rune, end_rune, metadata, 1 - (embedding <=> $1::vector) AS score FROM `)
	builder.WriteString(s.tableName)
	builder.WriteString(` WHERE 1 = 1`)

	argPos := 2
	if len(filter.SourcePaths) > 0 {
		_, _ = fmt.Fprintf(&builder, ` AND source_path = ANY($%d::text[])`, argPos)
		args = append(args, filter.SourcePaths)
		argPos++
	}
	if len(filter.SourcePrefixes) > 0 {
		prefixes := make([]string, 0, len(filter.SourcePrefixes))
		for _, prefix := range filter.SourcePrefixes {
			prefixes = append(prefixes, prefix+"%")
		}
		_, _ = fmt.Fprintf(&builder, ` AND source_path LIKE ANY($%d::text[])`, argPos)
		args = append(args, prefixes)
		argPos++
	}
	if len(filter.Metadata) > 0 {
		metadataJSON, err := marshalMetadataJSON(filter.Metadata)
		if err != nil {
			return "", nil, fmt.Errorf("marshal filter metadata: %w", err)
		}
		_, _ = fmt.Fprintf(&builder, ` AND metadata @> $%d::jsonb`, argPos)
		args = append(args, metadataJSON)
		argPos++
	}
	if threshold > -1 {
		_, _ = fmt.Fprintf(&builder, ` AND 1 - (embedding <=> $1::vector) >= $%d`, argPos)
		args = append(args, threshold)
		argPos++
	}
	builder.WriteString(` ORDER BY embedding <=> $1::vector`)
	_, _ = fmt.Fprintf(&builder, ` LIMIT $%d`, argPos)
	args = append(args, min(topK, math.MaxInt32))

	return builder.String(), args, nil
}

type pgxRows interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
	Close()
}

func (s *pgVectorStore) queryRows(ctx context.Context, sql string, args ...any) (pgxRows, error) {
	switch s.cfg.IndexStrategy {
	case pgVectorIndexHNSW:
		tx, err := s.pool.Begin(ctx)
		if err != nil {
			return nil, err
		}
		if _, err := tx.Exec(ctx, fmt.Sprintf(`SET LOCAL hnsw.ef_search = %d`, s.cfg.HNSWEfSearch)); err != nil {
			_ = tx.Rollback(ctx)
			return nil, err
		}
		rows, err := tx.Query(ctx, sql, args...)
		if err != nil {
			_ = tx.Rollback(ctx)
			return nil, err
		}
		return &txRows{tx: tx, rows: rows}, nil
	case pgVectorIndexIVFFlat:
		tx, err := s.pool.Begin(ctx)
		if err != nil {
			return nil, err
		}
		if _, err := tx.Exec(ctx, fmt.Sprintf(`SET LOCAL ivfflat.probes = %d`, s.cfg.IVFFlatProbes)); err != nil {
			_ = tx.Rollback(ctx)
			return nil, err
		}
		rows, err := tx.Query(ctx, sql, args...)
		if err != nil {
			_ = tx.Rollback(ctx)
			return nil, err
		}
		return &txRows{tx: tx, rows: rows}, nil
	default:
		return s.pool.Query(ctx, sql, args...)
	}
}

type txRows struct {
	tx   pgx.Tx
	rows pgx.Rows
}

func (r *txRows) Next() bool             { return r.rows.Next() }
func (r *txRows) Scan(dest ...any) error { return r.rows.Scan(dest...) }
func (r *txRows) Err() error             { return r.rows.Err() }
func (r *txRows) Close() {
	r.rows.Close()
	_ = r.tx.Commit(context.Background())
}

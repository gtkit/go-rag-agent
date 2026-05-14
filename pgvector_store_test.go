package ragagent

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gtkit/pgorm"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestPGVectorStoreConfigValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		cfg     PGVectorStoreConfig
		wantErr error
	}{
		{
			name: "valid conn string config",
			cfg: PGVectorStoreConfig{
				ConnString: "postgres://user:pass@127.0.0.1:5432/dbname?sslmode=disable",
				TableName:  "knowledge_chunks",
				Dimensions: 1536,
			},
			wantErr: nil,
		},
		{
			name: "requires pool or conn string",
			cfg: PGVectorStoreConfig{
				TableName:  "knowledge_chunks",
				Dimensions: 1536,
			},
			wantErr: ErrInvalidConfig,
		},
		{
			name: "requires table name",
			cfg: PGVectorStoreConfig{
				ConnString: "postgres://user:pass@127.0.0.1:5432/dbname?sslmode=disable",
				Dimensions: 1536,
			},
			wantErr: ErrInvalidConfig,
		},
		{
			name: "requires dimensions",
			cfg: PGVectorStoreConfig{
				ConnString: "postgres://user:pass@127.0.0.1:5432/dbname?sslmode=disable",
				TableName:  "knowledge_chunks",
			},
			wantErr: ErrInvalidConfig,
		},
		{
			name: "valid pgorm config",
			cfg: PGVectorStoreConfig{
				PGORMConfig: ptr(pgorm.NewConfig(
					pgorm.WithDSN("postgres://user:pass@127.0.0.1:5432/dbname?sslmode=disable"),
					pgorm.WithStartupPing(false),
				)),
				TableName:  "knowledge_chunks",
				Dimensions: 1536,
			},
			wantErr: nil,
		},
		{
			name: "rejects multiple connection sources",
			cfg: PGVectorStoreConfig{
				ConnString: "postgres://user:pass@127.0.0.1:5432/dbname?sslmode=disable",
				PGORMConfig: ptr(pgorm.NewConfig(
					pgorm.WithDSN("postgres://user:pass@127.0.0.1:5432/dbname?sslmode=disable"),
					pgorm.WithStartupPing(false),
				)),
				TableName:  "knowledge_chunks",
				Dimensions: 1536,
			},
			wantErr: ErrInvalidConfig,
		},
		{
			name: "rejects unsupported distance metric",
			cfg: PGVectorStoreConfig{
				ConnString:     "postgres://user:pass@127.0.0.1:5432/dbname?sslmode=disable",
				TableName:      "knowledge_chunks",
				Dimensions:     1536,
				DistanceMetric: "l2",
			},
			wantErr: ErrInvalidConfig,
		},
		{
			name: "rejects non-positive upsert batch size",
			cfg: PGVectorStoreConfig{
				ConnString:      "postgres://user:pass@127.0.0.1:5432/dbname?sslmode=disable",
				TableName:       "knowledge_chunks",
				Dimensions:      1536,
				UpsertBatchSize: -1,
			},
			wantErr: ErrInvalidConfig,
		},
		{
			name: "rejects unsupported index strategy",
			cfg: PGVectorStoreConfig{
				ConnString:    "postgres://user:pass@127.0.0.1:5432/dbname?sslmode=disable",
				TableName:     "knowledge_chunks",
				Dimensions:    1536,
				IndexStrategy: "diskann",
			},
			wantErr: ErrInvalidConfig,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := tc.cfg.validate()
			if tc.wantErr == nil {
				if err != nil {
					t.Fatalf("validate() error = %v", err)
				}
				return
			}
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("validate() error = %v, want errors.Is(..., %v)", err, tc.wantErr)
			}
		})
	}
}

func TestPGVectorStoreConfigNormalized(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		cfg  PGVectorStoreConfig
		want PGVectorStoreConfig
	}{
		{
			name: "applies defaults",
			cfg: PGVectorStoreConfig{
				ConnString: "postgres://user:pass@127.0.0.1:5432/dbname?sslmode=disable",
				TableName:  "knowledge_chunks",
				Dimensions: 1536,
			},
			want: PGVectorStoreConfig{
				ConnString:          "postgres://user:pass@127.0.0.1:5432/dbname?sslmode=disable",
				SchemaName:          "public",
				TableName:           "knowledge_chunks",
				Dimensions:          1536,
				DistanceMetric:      "cosine",
				AutoCreateExtension: true,
				AutoCreateSchema:    true,
				AutoCreateTable:     true,
				AutoCreateIndexes:   true,
				IndexStrategy:       "none",
				UpsertBatchSize:     200,
				HNSWM:               16,
				HNSWEfConstruction:  64,
				HNSWEfSearch:        100,
			},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := tc.cfg.normalized()
			if got.SchemaName != tc.want.SchemaName ||
				got.DistanceMetric != tc.want.DistanceMetric ||
				got.IndexStrategy != tc.want.IndexStrategy ||
				got.AutoCreateExtension != tc.want.AutoCreateExtension ||
				got.AutoCreateSchema != tc.want.AutoCreateSchema ||
				got.AutoCreateTable != tc.want.AutoCreateTable ||
				got.AutoCreateIndexes != tc.want.AutoCreateIndexes ||
				got.UpsertBatchSize != tc.want.UpsertBatchSize ||
				got.HNSWM != tc.want.HNSWM ||
				got.HNSWEfConstruction != tc.want.HNSWEfConstruction ||
				got.HNSWEfSearch != tc.want.HNSWEfSearch {
				t.Fatalf("normalized() = %#v, want %#v", got, tc.want)
			}
		})
	}
}

func TestPGVectorStoreCreateTableAndIndexSQL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		cfg      PGVectorStoreConfig
		wantAny  []string
		wantIdx  []string
		wantMiss []string
	}{
		{
			name: "default exact search table and base indexes",
			cfg: PGVectorStoreConfig{
				ConnString: "postgres://user:pass@127.0.0.1:5432/dbname?sslmode=disable",
				TableName:  "knowledge_chunks",
				Dimensions: 1536,
			},
			wantAny: []string{
				`CREATE TABLE IF NOT EXISTS "public"."knowledge_chunks"`,
				`embedding vector(1536) NOT NULL`,
			},
			wantIdx: []string{
				`parent_id`,
				`source_path`,
				`jsonb_path_ops`,
			},
			wantMiss: []string{
				`USING hnsw`,
				`USING ivfflat`,
			},
		},
		{
			name: "hnsw index enabled",
			cfg: PGVectorStoreConfig{
				ConnString:    "postgres://user:pass@127.0.0.1:5432/dbname?sslmode=disable",
				TableName:     "knowledge_chunks",
				Dimensions:    1536,
				IndexStrategy: pgVectorIndexHNSW,
			},
			wantAny: []string{},
			wantIdx: []string{
				`vector_cosine_ops`,
				`USING hnsw`,
				`ef_construction = 64`,
			},
		},
		{
			name: "ivfflat index enabled",
			cfg: PGVectorStoreConfig{
				ConnString:    "postgres://user:pass@127.0.0.1:5432/dbname?sslmode=disable",
				TableName:     "knowledge_chunks",
				Dimensions:    1536,
				IndexStrategy: pgVectorIndexIVFFlat,
			},
			wantIdx: []string{
				`vector_cosine_ops`,
				`USING ivfflat`,
				`lists = 100`,
			},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			store := &pgVectorStore{
				cfg:       tc.cfg.normalized(),
				tableName: `"public"."knowledge_chunks"`,
			}
			tableSQL := store.createTableSQL()
			for _, want := range tc.wantAny {
				if !strings.Contains(tableSQL, want) {
					t.Fatalf("createTableSQL() = %q, missing %q", tableSQL, want)
				}
			}

			idxSQL := strings.Join(store.indexStatements(), "\n")
			for _, want := range tc.wantIdx {
				if !strings.Contains(idxSQL, want) {
					t.Fatalf("indexStatements() = %q, missing %q", idxSQL, want)
				}
			}
			for _, miss := range tc.wantMiss {
				if strings.Contains(idxSQL, miss) {
					t.Fatalf("indexStatements() = %q, unexpected %q", idxSQL, miss)
				}
			}
		})
	}
}

func TestPGVectorStoreValidateChunkAndHelpers(t *testing.T) {
	t.Parallel()

	store := &pgVectorStore{
		cfg: PGVectorStoreConfig{
			Dimensions: 3,
		}.normalized(),
	}
	tests := []struct {
		name    string
		chunk   ChunkRecord
		wantErr string
	}{
		{
			name:    "rejects missing chunk id",
			chunk:   ChunkRecord{ParentID: "doc", Embedding: []float32{1, 2, 3}},
			wantErr: "chunk id is empty",
		},
		{
			name:    "rejects missing parent id",
			chunk:   ChunkRecord{ChunkID: "doc:0", Embedding: []float32{1, 2, 3}},
			wantErr: "parent id is empty",
		},
		{
			name:    "rejects dimension mismatch",
			chunk:   ChunkRecord{ChunkID: "doc:0", ParentID: "doc", Embedding: []float32{1}},
			wantErr: "embedding dimensions",
		},
		{
			name:  "accepts valid chunk",
			chunk: ChunkRecord{ChunkID: "doc:0", ParentID: "doc", Embedding: []float32{1, 2, 3}},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := store.validateChunk(tt.chunk)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("validateChunk() error = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("validateChunk() error = %v", err)
			}
		})
	}

	data, err := marshalMetadataJSON(map[string]string{"team": "search"})
	if err != nil {
		t.Fatalf("marshalMetadataJSON() error = %v", err)
	}
	if got := string(data); !strings.Contains(got, `"team":"search"`) {
		t.Fatalf("marshalMetadataJSON() = %s, want team metadata", got)
	}
	empty, err := marshalMetadataJSON(nil)
	if err != nil {
		t.Fatalf("marshalMetadataJSON(nil) error = %v", err)
	}
	if string(empty) != `{}` {
		t.Fatalf("marshalMetadataJSON(nil) = %s, want {}", empty)
	}
}

func TestPGVectorStoreDuplicateExtensionRace(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "matches duplicate extension name index",
			err:  &pgconn.PgError{Code: "23505", Message: "duplicate key value violates unique constraint \"pg_extension_name_index\""},
			want: true,
		},
		{
			name: "other unique violation is not extension race",
			err:  &pgconn.PgError{Code: "23505", Message: "other unique constraint"},
			want: false,
		},
		{
			name: "plain error is not extension race",
			err:  errors.New("boom"),
			want: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := isDuplicateExtensionRace(tt.err); got != tt.want {
				t.Fatalf("isDuplicateExtensionRace() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPGVectorStoreBuildSearchSQL(t *testing.T) {
	t.Parallel()

	store := &pgVectorStore{
		cfg: PGVectorStoreConfig{
			SchemaName:     "public",
			TableName:      "knowledge_chunks",
			Dimensions:     3,
			DistanceMetric: pgVectorDistanceCosine,
			IndexStrategy:  pgVectorIndexNone,
		}.normalized(),
		tableName: `"public"."knowledge_chunks"`,
	}

	tests := []struct {
		name      string
		filter    SearchFilter
		threshold float32
		wantAny   []string
		wantArgs  int
	}{
		{
			name:      "plain search",
			filter:    SearchFilter{},
			threshold: 0,
			wantAny: []string{
				`1 - (embedding <=> $1::vector) AS score`,
				`ORDER BY embedding <=> $1::vector`,
				`LIMIT $3`,
			},
			wantArgs: 3,
		},
		{
			name: "with full filter",
			filter: SearchFilter{
				SourcePaths:    []string{"/kb/a.md"},
				SourcePrefixes: []string{"/kb"},
				Metadata:       map[string]string{"tag": "api"},
			},
			threshold: 0.5,
			wantAny: []string{
				`source_path = ANY($2::text[])`,
				`source_path LIKE ANY($3::text[])`,
				`metadata @> $4::jsonb`,
				`1 - (embedding <=> $1::vector) >= $5`,
				`LIMIT $6`,
			},
			wantArgs: 6,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			sql, args, err := store.buildSearchSQL([]float32{1, 0, 0}, 8, tc.threshold, tc.filter)
			if err != nil {
				t.Fatalf("buildSearchSQL() error = %v", err)
			}
			for _, want := range tc.wantAny {
				if !strings.Contains(sql, want) {
					t.Fatalf("buildSearchSQL() = %q, missing %q", sql, want)
				}
			}
			if len(args) != tc.wantArgs {
				t.Fatalf("buildSearchSQL() arg len = %d, want %d", len(args), tc.wantArgs)
			}
		})
	}
}

func TestPGVectorStoreIntegration(t *testing.T) {
	t.Parallel()

	dsn := os.Getenv("RAGAGENT_PGVECTOR_TEST_DSN")
	if strings.TrimSpace(dsn) == "" {
		t.Skip("RAGAGENT_PGVECTOR_TEST_DSN is not set")
	}

	tests := []struct {
		name       string
		newStore   func(t *testing.T) VectorStore
		afterClose func(t *testing.T)
		run        func(t *testing.T, store VectorStore)
	}{
		{
			name: "upsert search and delete by source paths",
			newStore: func(t *testing.T) VectorStore {
				t.Helper()
				store, err := NewPGVectorStore(PGVectorStoreConfig{
					ConnString: dsn,
					TableName:  pgVectorTestTableName(t.Name()),
					Dimensions: 3,
				})
				if err != nil {
					t.Fatalf("NewPGVectorStore() error = %v", err)
				}
				return store
			},
			run: func(t *testing.T, store VectorStore) {
				t.Helper()

				err := store.Upsert(context.Background(), []ChunkRecord{
					{
						ChunkID:    "doc-a:0",
						ParentID:   "doc-a",
						SourcePath: "/kb/a.md",
						Title:      "A",
						Text:       "gateway api",
						StartRune:  0,
						EndRune:    11,
						Metadata:   map[string]string{"tag": "api"},
						Embedding:  []float32{1, 0, 0},
					},
					{
						ChunkID:    "doc-b:0",
						ParentID:   "doc-b",
						SourcePath: "/kb/b.md",
						Title:      "B",
						Text:       "redis cache",
						StartRune:  0,
						EndRune:    11,
						Metadata:   map[string]string{"tag": "infra"},
						Embedding:  []float32{0, 1, 0},
					},
				})
				if err != nil {
					t.Fatalf("Upsert() error = %v", err)
				}

				hits, err := store.SearchWithFilter(context.Background(), []float32{1, 0, 0}, 1, 0, SearchFilter{
					Metadata: map[string]string{"tag": "api"},
				})
				if err != nil {
					t.Fatalf("SearchWithFilter() error = %v", err)
				}
				if len(hits) != 1 {
					t.Fatalf("SearchWithFilter() len = %d, want 1", len(hits))
				}
				if hits[0].Chunk.ChunkID != "doc-a:0" {
					t.Fatalf("SearchWithFilter() top chunk = %q, want %q", hits[0].Chunk.ChunkID, "doc-a:0")
				}

				if err := store.DeleteBySourcePaths(context.Background(), []string{"/kb/a.md"}); err != nil {
					t.Fatalf("DeleteBySourcePaths() error = %v", err)
				}
			},
		},
		{
			name: "pgorm config connection source",
			newStore: func(t *testing.T) VectorStore {
				t.Helper()
				pgormCfg := pgorm.NewConfig(
					pgorm.WithDSN(dsn),
					pgorm.WithStartupPing(false),
				)
				store, err := NewPGVectorStore(PGVectorStoreConfig{
					PGORMConfig: &pgormCfg,
					TableName:   pgVectorTestTableName(t.Name()),
					Dimensions:  3,
				})
				if err != nil {
					t.Fatalf("NewPGVectorStore() error = %v", err)
				}
				return store
			},
			run: func(t *testing.T, store VectorStore) {
				t.Helper()
				if err := store.Upsert(context.Background(), []ChunkRecord{
					{
						ChunkID:    "doc-a:0",
						ParentID:   "doc-a",
						SourcePath: "/kb/a.md",
						Title:      "A",
						Text:       "gateway api",
						StartRune:  0,
						EndRune:    11,
						Metadata:   map[string]string{"tag": "api"},
						Embedding:  []float32{1, 0, 0},
					},
				}); err != nil {
					t.Fatalf("Upsert() error = %v", err)
				}
				hits, err := store.Search(context.Background(), []float32{1, 0, 0}, 1, 0)
				if err != nil {
					t.Fatalf("Search() error = %v", err)
				}
				if len(hits) != 1 {
					t.Fatalf("Search() len = %d, want 1", len(hits))
				}
			},
		},
		{
			name: "pgorm client connection source keeps external client open",
			newStore: func(t *testing.T) VectorStore {
				t.Helper()
				client, err := pgorm.Open(context.Background(),
					pgorm.WithDSN(dsn),
					pgorm.WithStartupPing(false),
				)
				if err != nil {
					t.Fatalf("pgorm.Open() error = %v", err)
				}
				t.Cleanup(client.Close)
				store, err := NewPGVectorStore(PGVectorStoreConfig{
					PGORMClient: client,
					TableName:   pgVectorTestTableName(t.Name()),
					Dimensions:  3,
				})
				if err != nil {
					t.Fatalf("NewPGVectorStore() error = %v", err)
				}
				t.Cleanup(func() {
					if pingErr := client.PingContext(context.Background()); pingErr != nil {
						t.Fatalf("pgorm client ping after store close = %v", pingErr)
					}
				})
				return store
			},
			run: func(t *testing.T, store VectorStore) {
				t.Helper()
				if err := store.Upsert(context.Background(), []ChunkRecord{
					{
						ChunkID:    "doc-a:0",
						ParentID:   "doc-a",
						SourcePath: "/kb/a.md",
						Title:      "A",
						Text:       "gateway api",
						StartRune:  0,
						EndRune:    11,
						Metadata:   map[string]string{"tag": "api"},
						Embedding:  []float32{1, 0, 0},
					},
				}); err != nil {
					t.Fatalf("Upsert() error = %v", err)
				}
			},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			store := tc.newStore(t)
			t.Cleanup(func() {
				if cerr := store.Close(); cerr != nil {
					t.Fatalf("Close() error = %v", cerr)
				}
			})

			tc.run(t, store)
		})
	}
}

func TestAgentWithPGVectorStoreIntegration(t *testing.T) {
	t.Parallel()

	dsn := os.Getenv("RAGAGENT_PGVECTOR_TEST_DSN")
	if strings.TrimSpace(dsn) == "" {
		t.Skip("RAGAGENT_PGVECTOR_TEST_DSN is not set")
	}

	store, err := NewPGVectorStore(PGVectorStoreConfig{
		ConnString: dsn,
		TableName:  pgVectorTestTableName(t.Name()),
		Dimensions: 3,
	})
	if err != nil {
		t.Fatalf("NewPGVectorStore() error = %v", err)
	}
	t.Cleanup(func() {
		if cerr := store.Close(); cerr != nil {
			t.Fatalf("Close() error = %v", cerr)
		}
	})

	root := t.TempDir()
	path := root + "/knowledge.md"
	if err := os.WriteFile(path, []byte("# Doc\n\ngateway api exact match"), 0o600); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", path, err)
	}

	agent, err := New(Config{
		Runtime: RuntimeComponents{
			ChatModel: stubRuntimeChatModel{},
			Embedder:  stubPGVectorEmbedder{},
		},
		Storage: StorageComponents{
			VectorStore:    store,
			DocumentLoader: NewFileDocumentLoader(),
			Reranker:       NewRuleBasedReranker(),
		},
		TopK:                      1,
		ChunkSize:                 64,
		ChunkOverlap:              0,
		MaxHistoryRounds:          8,
		RequestTimeout:            time.Second,
		EnableHybridSearch:        true,
		EnableRerank:              true,
		HybridCandidateMultiplier: 1,
		HybridRRFK:                60,
		RerankShortlistMultiplier: 1,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	t.Cleanup(func() {
		if cerr := agent.Close(); cerr != nil {
			t.Fatalf("Close() error = %v", cerr)
		}
	})

	if err := agent.AddKnowledge(context.Background(), FileSource(path)); err != nil {
		t.Fatalf("AddKnowledge() error = %v", err)
	}
	answer, err := agent.GetSession("pgvector").Ask(context.Background(), "gateway api")
	if err != nil {
		t.Fatalf("Ask() error = %v", err)
	}
	if answer.Text == "" {
		t.Fatal("Ask() returned empty text")
	}
}

type stubPGVectorEmbedder struct{}

func (stubPGVectorEmbedder) EmbedTexts(_ context.Context, texts []string) ([][]float32, error) {
	rows := make([][]float32, 0, len(texts))
	for _, text := range texts {
		switch {
		case strings.Contains(text, "gateway api"):
			rows = append(rows, []float32{1, 0, 0})
		default:
			rows = append(rows, []float32{0, 1, 0})
		}
	}
	return rows, nil
}

func pgVectorTestTableName(name string) string {
	replacer := strings.NewReplacer("/", "_", "-", "_", " ", "_")
	return "pgvector_" + strings.ToLower(replacer.Replace(name))
}

func ptr[T any](v T) *T {
	return &v
}

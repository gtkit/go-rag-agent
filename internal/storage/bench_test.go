package storage

import (
	"context"
	"fmt"
	"testing"
)

func BenchmarkChromemStoreSearchInMemory(b *testing.B) {
	ctx := context.Background()

	store, err := NewChromemStore(Config{DataDir: "", Collection: "bench"})
	if err != nil {
		b.Fatalf("new store: %v", err)
	}
	defer func() {
		_ = store.Close()
	}()

	const (
		chunkCount = 500
		embedDim   = 128
		topK       = 8
	)

	records := make([]ChunkRecord, 0, chunkCount)
	for i := 0; i < chunkCount; i++ {
		records = append(records, ChunkRecord{
			ChunkID:    fmt.Sprintf("chunk-%04d", i),
			ParentID:   fmt.Sprintf("doc-%03d", i/10),
			SourcePath: "bench.md",
			Title:      "bench",
			Text:       fmt.Sprintf("benchmark content %d", i),
			StartRune:  i * 10,
			EndRune:    i*10 + 9,
			Embedding:  benchmarkEmbedding(i, embedDim),
		})
	}

	if err := store.Upsert(ctx, records); err != nil {
		b.Fatalf("upsert: %v", err)
	}

	query := benchmarkEmbedding(7, embedDim)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		hits, err := store.Search(ctx, query, topK, 0)
		if err != nil {
			b.Fatalf("search: %v", err)
		}
		if len(hits) == 0 {
			b.Fatal("expected non-empty hits")
		}
	}
}

func benchmarkEmbedding(seed int, dim int) []float32 {
	row := make([]float32, dim)
	for i := 0; i < dim; i++ {
		row[i] = float32((seed%31)+1) * float32((i%11)+1) / 100
	}
	return row
}

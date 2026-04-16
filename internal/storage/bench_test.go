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
	state := uint64(seed+1)*0x9e3779b97f4a7c15 + 0xbf58476d1ce4e5b9
	for i := 0; i < dim; i++ {
		state ^= state >> 30
		state *= 0xbf58476d1ce4e5b9
		state ^= state >> 27
		state *= 0x94d049bb133111eb
		state ^= state >> 31

		// Deterministic pseudo-random values in [-1, 1], non-collinear across seeds.
		row[i] = float32(int64(state%2001)-1000) / 1000
	}
	return row
}

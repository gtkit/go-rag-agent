package rag

import (
	"fmt"
	"strings"
	"testing"
)

func BenchmarkChunkerSplit(b *testing.B) {
	chunker, err := NewChunker(800, 120)
	if err != nil {
		b.Fatalf("new chunker: %v", err)
	}

	doc := Document{
		ID:         "bench-doc",
		SourcePath: "bench.md",
		Title:      "bench",
		Metadata:   map[string]string{"env": "bench"},
		Content:    strings.Repeat("这是一个用于分块基准测试的段落。", 3000),
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		chunks := chunker.Split(doc)
		if len(chunks) == 0 {
			b.Fatal("expected chunks")
		}
	}
}

func BenchmarkAssembleContext(b *testing.B) {
	chunks := make([]Chunk, 0, 64)
	for i := 0; i < 64; i++ {
		chunks = append(chunks, Chunk{
			ChunkID: fmt.Sprintf("chunk-%d", i),
			Text:    strings.Repeat("context-segment ", 20),
		})
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		kept, contextText, err := AssembleContext(chunks, 4000)
		if err != nil {
			b.Fatalf("assemble context: %v", err)
		}
		if len(kept) == 0 || contextText == "" {
			b.Fatal("expected non-empty context")
		}
	}
}

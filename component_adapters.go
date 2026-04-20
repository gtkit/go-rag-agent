package ragagent

import (
	"maps"
	"slices"

	"github.com/gtkit/go-rag-agent/internal/rag"
	"github.com/gtkit/go-rag-agent/internal/storage"
)

func fromInternalDocument(doc rag.Document) Document {
	return Document{
		ID:         doc.ID,
		SourcePath: doc.SourcePath,
		Title:      doc.Title,
		Metadata:   maps.Clone(doc.Metadata),
		Content:    doc.Content,
	}
}

func toInternalDocument(doc Document) rag.Document {
	return rag.Document{
		ID:         doc.ID,
		SourcePath: doc.SourcePath,
		Title:      doc.Title,
		Metadata:   maps.Clone(doc.Metadata),
		Content:    doc.Content,
	}
}

func fromInternalChunkRecords(chunks []storage.ChunkRecord) []ChunkRecord {
	if len(chunks) == 0 {
		return nil
	}

	out := make([]ChunkRecord, 0, len(chunks))
	for _, chunk := range chunks {
		out = append(out, fromInternalChunkRecord(chunk))
	}
	return out
}

func toInternalChunkRecords(chunks []ChunkRecord) []storage.ChunkRecord {
	if len(chunks) == 0 {
		return nil
	}

	out := make([]storage.ChunkRecord, 0, len(chunks))
	for _, chunk := range chunks {
		out = append(out, storage.ChunkRecord{
			ChunkID:    chunk.ChunkID,
			ParentID:   chunk.ParentID,
			SourcePath: chunk.SourcePath,
			Title:      chunk.Title,
			Text:       chunk.Text,
			StartRune:  chunk.StartRune,
			EndRune:    chunk.EndRune,
			Metadata:   maps.Clone(chunk.Metadata),
			Embedding:  slices.Clone(chunk.Embedding),
		})
	}
	return out
}

func fromInternalChunkRecord(chunk storage.ChunkRecord) ChunkRecord {
	return ChunkRecord{
		ChunkID:    chunk.ChunkID,
		ParentID:   chunk.ParentID,
		SourcePath: chunk.SourcePath,
		Title:      chunk.Title,
		Text:       chunk.Text,
		StartRune:  chunk.StartRune,
		EndRune:    chunk.EndRune,
		Metadata:   maps.Clone(chunk.Metadata),
		Embedding:  slices.Clone(chunk.Embedding),
	}
}

func toInternalSearchFilter(filter SearchFilter) storage.SearchFilter {
	return storage.SearchFilter{
		SourcePaths:    slices.Clone(filter.SourcePaths),
		SourcePrefixes: slices.Clone(filter.SourcePrefixes),
		Metadata:       maps.Clone(filter.Metadata),
	}
}

func fromInternalSearchFilter(filter storage.SearchFilter) SearchFilter {
	return SearchFilter{
		SourcePaths:    slices.Clone(filter.SourcePaths),
		SourcePrefixes: slices.Clone(filter.SourcePrefixes),
		Metadata:       maps.Clone(filter.Metadata),
	}
}

func toInternalSearchHits(hits []SearchHit) []storage.SearchHit {
	if len(hits) == 0 {
		return nil
	}

	out := make([]storage.SearchHit, 0, len(hits))
	for _, hit := range hits {
		out = append(out, storage.SearchHit{
			Chunk: fromRootChunkRecord(hit.Chunk),
			Score: hit.Score,
		})
	}
	return out
}

func fromInternalSearchHits(hits []storage.SearchHit) []SearchHit {
	if len(hits) == 0 {
		return nil
	}

	out := make([]SearchHit, 0, len(hits))
	for _, hit := range hits {
		out = append(out, SearchHit{
			Chunk: fromInternalChunkRecord(hit.Chunk),
			Score: hit.Score,
		})
	}
	return out
}

func fromRootChunkRecord(chunk ChunkRecord) storage.ChunkRecord {
	return storage.ChunkRecord{
		ChunkID:    chunk.ChunkID,
		ParentID:   chunk.ParentID,
		SourcePath: chunk.SourcePath,
		Title:      chunk.Title,
		Text:       chunk.Text,
		StartRune:  chunk.StartRune,
		EndRune:    chunk.EndRune,
		Metadata:   maps.Clone(chunk.Metadata),
		Embedding:  slices.Clone(chunk.Embedding),
	}
}

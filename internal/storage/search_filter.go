package storage

import (
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

func (f SearchFilter) normalized() SearchFilter {
	return SearchFilter{
		SourcePaths:    normalizeSearchFilterPaths(f.SourcePaths),
		SourcePrefixes: normalizeSearchFilterPaths(f.SourcePrefixes),
		Metadata:       normalizeSearchFilterMetadata(f.Metadata),
	}
}

func (f SearchFilter) isZero() bool {
	return len(f.SourcePaths) == 0 && len(f.SourcePrefixes) == 0 && len(f.Metadata) == 0
}

func (f SearchFilter) matchesChunk(chunk ChunkRecord) bool {
	if len(f.SourcePaths) > 0 && !slices.Contains(f.SourcePaths, filepath.Clean(chunk.SourcePath)) {
		return false
	}
	if len(f.SourcePrefixes) > 0 {
		matchedPrefix := false
		for _, prefix := range f.SourcePrefixes {
			if sourcePathMatchesPrefix(chunk.SourcePath, prefix) {
				matchedPrefix = true
				break
			}
		}
		if !matchedPrefix {
			return false
		}
	}
	for key, value := range f.Metadata {
		if chunk.Metadata[key] != value {
			return false
		}
	}
	return true
}

func normalizeSearchFilterPaths(paths []string) []string {
	if len(paths) == 0 {
		return nil
	}

	normalized := make([]string, 0, len(paths))
	for _, path := range paths {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		normalized = append(normalized, filepath.Clean(path))
	}
	if len(normalized) == 0 {
		return nil
	}

	slices.Sort(normalized)
	return slices.Compact(normalized)
}

func normalizeSearchFilterMetadata(metadata map[string]string) map[string]string {
	if len(metadata) == 0 {
		return nil
	}

	cloned := maps.Clone(metadata)
	for key, value := range cloned {
		trimmedKey := strings.TrimSpace(key)
		if trimmedKey == "" {
			delete(cloned, key)
			continue
		}
		if trimmedKey != key {
			delete(cloned, key)
		}
		cloned[trimmedKey] = strings.TrimSpace(value)
	}
	if len(cloned) == 0 {
		return nil
	}
	return cloned
}

func sourcePathMatchesPrefix(sourcePath string, prefix string) bool {
	sourcePath = filepath.Clean(sourcePath)
	prefix = filepath.Clean(prefix)
	if sourcePath == prefix {
		return true
	}
	separatorSuffix := prefix + string(os.PathSeparator)
	if prefix == string(os.PathSeparator) {
		separatorSuffix = prefix
	}
	return strings.HasPrefix(sourcePath, separatorSuffix)
}

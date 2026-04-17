package ragagent

import (
	"maps"
	"path/filepath"
	"slices"
	"strings"

	"my-gtkit-package/go-rag-agent/internal/storage"
)

func (o QueryOptions) storageFilter() storage.SearchFilter {
	return storage.SearchFilter{
		SourcePaths:    normalizeFilterPaths(o.Filter.SourcePaths),
		SourcePrefixes: normalizeFilterPaths(o.Filter.SourcePrefixes),
		Metadata:       normalizeFilterMetadata(o.Filter.Metadata),
	}
}

func normalizeFilterPaths(paths []string) []string {
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

func normalizeFilterMetadata(metadata map[string]string) map[string]string {
	if len(metadata) == 0 {
		return nil
	}

	cloned := maps.Clone(metadata)
	for key, value := range cloned {
		key = strings.TrimSpace(key)
		if key == "" {
			delete(cloned, key)
			continue
		}
		cloned[key] = strings.TrimSpace(value)
	}
	if len(cloned) == 0 {
		return nil
	}
	return cloned
}

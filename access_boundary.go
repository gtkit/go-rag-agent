package ragagent

import (
	"maps"
	"slices"
	"strings"

	"github.com/gtkit/go-rag-agent/internal/storage"
)

const accessBoundaryNamespaceKey = "rag_namespace"
const accessBoundaryNoMatchSource = "/__ragagent_no_match__"

// AccessBoundaryConfig 定义静态租户与来源权限边界。
type AccessBoundaryConfig struct {
	Namespace             string
	AllowedSourcePaths    []string
	AllowedSourcePrefixes []string
}

func (c AccessBoundaryConfig) normalized() AccessBoundaryConfig {
	c.Namespace = strings.TrimSpace(c.Namespace)
	c.AllowedSourcePaths = normalizeStringSlice(c.AllowedSourcePaths)
	c.AllowedSourcePrefixes = normalizeStringSlice(c.AllowedSourcePrefixes)
	return c
}

func normalizeStringSlice(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, 0, len(in))
	for _, item := range in {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			continue
		}
		out = append(out, trimmed)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func applyNamespaceMetadata(metadata map[string]string, namespace string) map[string]string {
	if strings.TrimSpace(namespace) == "" {
		return metadata
	}
	out := maps.Clone(metadata)
	if out == nil {
		out = make(map[string]string, 1)
	}
	out[accessBoundaryNamespaceKey] = namespace
	return out
}

func (c AccessBoundaryConfig) applyToFilter(filter storage.SearchFilter) storage.SearchFilter {
	c = c.normalized()
	filter.Metadata = maps.Clone(filter.Metadata)
	if c.Namespace != "" {
		if filter.Metadata == nil {
			filter.Metadata = make(map[string]string, 1)
		}
		filter.Metadata[accessBoundaryNamespaceKey] = c.Namespace
	}
	filter.SourcePaths = mergeAllowedPaths(filter.SourcePaths, c.AllowedSourcePaths)
	filter.SourcePrefixes = mergeAllowedPrefixes(filter.SourcePrefixes, c.AllowedSourcePrefixes)
	return filter
}

func mergeAllowedPaths(current []string, allowed []string) []string {
	if len(allowed) == 0 {
		return slices.Clone(current)
	}
	if len(current) == 0 {
		return slices.Clone(allowed)
	}
	out := make([]string, 0, len(current))
	for _, path := range current {
		if slices.Contains(allowed, path) {
			out = append(out, path)
		}
	}
	if len(out) == 0 {
		return []string{accessBoundaryNoMatchSource}
	}
	return out
}

func mergeAllowedPrefixes(current []string, allowed []string) []string {
	if len(allowed) == 0 {
		return slices.Clone(current)
	}
	if len(current) == 0 {
		return slices.Clone(allowed)
	}
	out := make([]string, 0, len(current))
	for _, prefix := range current {
		for _, allowedPrefix := range allowed {
			if strings.HasPrefix(prefix, allowedPrefix) || strings.HasPrefix(allowedPrefix, prefix) {
				out = append(out, prefix)
				break
			}
		}
	}
	if len(out) == 0 {
		return []string{accessBoundaryNoMatchSource}
	}
	return out
}

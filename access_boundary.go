package ragagent

import (
	"context"
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
	Policy                AccessPolicy
}

// AccessPolicyRequest 描述一次动态 access policy 请求。
type AccessPolicyRequest struct {
	SessionID   string
	Query       string
	Filter      RetrievalFilter
	MemoryScope MemoryScope
}

// AccessPolicy 定义 access boundary 的动态 hook。
type AccessPolicy interface {
	TransformKnowledgeFile(ctx context.Context, file KnowledgeFile) (KnowledgeFile, error)
	ConstrainRetrieval(ctx context.Context, req AccessPolicyRequest) (RetrievalFilter, error)
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

func (c AccessBoundaryConfig) applyToKnowledgeFile(ctx context.Context, file KnowledgeFile) (KnowledgeFile, error) {
	c = c.normalized()
	if c.Policy != nil {
		next, err := c.Policy.TransformKnowledgeFile(ctx, file)
		if err != nil {
			return KnowledgeFile{}, err
		}
		file = next
	}
	file.Metadata = applyNamespaceMetadata(file.Metadata, c.Namespace)
	return file, nil
}

func (c AccessBoundaryConfig) applyToRetrieval(ctx context.Context, req AccessPolicyRequest) (storage.SearchFilter, error) {
	c = c.normalized()
	filter := c.applyStaticToRootFilter(req.Filter)
	req.Filter = filter
	if c.Policy != nil {
		next, err := c.Policy.ConstrainRetrieval(ctx, req)
		if err != nil {
			return storage.SearchFilter{}, err
		}
		filter = c.applyStaticToRootFilter(next)
	}
	return toInternalRetrievalFilter(filter), nil
}

func (c AccessBoundaryConfig) applyStaticToRootFilter(filter RetrievalFilter) RetrievalFilter {
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

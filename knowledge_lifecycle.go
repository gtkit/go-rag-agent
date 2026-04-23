package ragagent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

// RemoveKnowledge 删除一个知识源当前已导入的内容。
func (a *Agent) RemoveKnowledge(ctx context.Context, src KnowledgeSource) error {
	if err := a.beginOperation(); err != nil {
		return err
	}
	defer a.endOperation()

	if err := ctx.Err(); err != nil {
		return err
	}
	if src == nil {
		return fmt.Errorf("knowledge source is nil: %w", ErrUnsupportedSource)
	}
	if closer, ok := src.(interface{ Close() error }); ok {
		defer func() {
			_ = closer.Close()
		}()
	}

	if root, ok := knowledgeDirectoryRoot(src); ok {
		dirSync, err := a.ensureDirectorySync()
		if err != nil {
			return err
		}
		currentPaths, err := resolveKnowledgeSourcePaths(ctx, src)
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		return dirSync.Remove(root, currentPaths, func(sourcePaths []string) error {
			return a.deleteKnowledgeSourcePaths(ctx, sourcePaths)
		})
	}

	sourcePaths, err := resolveKnowledgeSourcePaths(ctx, src)
	if err != nil {
		return err
	}
	return a.deleteKnowledgeSourcePaths(ctx, sourcePaths)
}

// RebuildKnowledge 先删除一个知识源的历史内容，再导入当前内容。
func (a *Agent) RebuildKnowledge(ctx context.Context, src KnowledgeSource) error {
	if err := a.RemoveKnowledge(ctx, src); err != nil {
		return err
	}
	return a.AddKnowledge(ctx, src)
}

func resolveKnowledgeSourcePaths(ctx context.Context, src KnowledgeSource) ([]string, error) {
	switch source := src.(type) {
	case fileSource:
		return []string{filepath.Clean(source.path)}, nil
	case *fileSource:
		return []string{filepath.Clean(source.path)}, nil
	}

	files, err := src.Resolve(ctx)
	if err != nil {
		return nil, err
	}
	sourcePaths := make([]string, 0, len(files))
	for _, file := range files {
		sourcePaths = append(sourcePaths, filepath.Clean(file.Path))
	}
	return normalizeDirectorySyncPaths(sourcePaths), nil
}

func (a *Agent) deleteKnowledgeSourcePaths(ctx context.Context, sourcePaths []string) error {
	if len(sourcePaths) == 0 {
		return nil
	}
	if err := a.store.DeleteBySourcePaths(ctx, sourcePaths); err != nil {
		return fmt.Errorf("delete knowledge source paths: %w", err)
	}
	return nil
}

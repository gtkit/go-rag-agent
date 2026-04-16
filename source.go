package ragagent

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

type fileSource struct {
	path string
}

func FileSource(path string) KnowledgeSource {
	return fileSource{path: path}
}

func (s fileSource) Resolve(ctx context.Context) ([]KnowledgeFile, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !isSupportedKnowledgePath(s.path) {
		return nil, fmt.Errorf("unsupported file extension for %q: %w", s.path, ErrUnsupportedSource)
	}
	info, err := os.Stat(s.path)
	if err != nil {
		return nil, fmt.Errorf("stat source file %q: %w", s.path, err)
	}
	if info.IsDir() {
		return nil, fmt.Errorf("file source is directory %q: %w", s.path, ErrUnsupportedSource)
	}
	return []KnowledgeFile{
		{
			Path:     s.path,
			Title:    titleFromPath(s.path),
			Metadata: map[string]string{},
		},
	}, nil
}

type dirSource struct {
	path string
}

func DirSource(path string) KnowledgeSource {
	return dirSource{path: path}
}

func (s dirSource) Resolve(ctx context.Context) ([]KnowledgeFile, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	files := make([]KnowledgeFile, 0)
	err := filepath.WalkDir(s.path, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !isSupportedKnowledgePath(path) {
			return nil
		}
		files = append(files, KnowledgeFile{
			Path:     path,
			Title:    titleFromPath(path),
			Metadata: map[string]string{},
		})
		return nil
	})
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
		return nil, fmt.Errorf("walk source dir %q: %w", s.path, err)
	}
	slices.SortFunc(files, func(a, b KnowledgeFile) int {
		return strings.Compare(a.Path, b.Path)
	})
	return files, nil
}

func isSupportedKnowledgePath(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".txt", ".md":
		return true
	default:
		return false
	}
}

func titleFromPath(path string) string {
	base := filepath.Base(path)
	ext := filepath.Ext(base)
	return strings.TrimSuffix(base, ext)
}

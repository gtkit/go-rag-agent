package ragagent

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
)

const directorySyncManifestName = "directory-sync-manifest.json"

type directorySyncManifest struct {
	Roots map[string][]string `json:"roots"`
}

// directorySyncState 保存目录导入的最近一次成功状态，用于删除感知同步。
type directorySyncState struct {
	mu           sync.Mutex
	manifestPath string
	roots        map[string][]string
}

func newDirectorySyncState(dataDir string) (*directorySyncState, error) {
	state := &directorySyncState{
		roots: make(map[string][]string),
	}
	if strings.TrimSpace(dataDir) == "" {
		return state, nil
	}

	state.manifestPath = filepath.Join(dataDir, directorySyncManifestName)
	raw, err := os.ReadFile(state.manifestPath)
	if err != nil {
		if os.IsNotExist(err) {
			return state, nil
		}
		return nil, fmt.Errorf("read directory sync manifest %q: %w", state.manifestPath, err)
	}

	var manifest directorySyncManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return nil, fmt.Errorf("unmarshal directory sync manifest %q: %w", state.manifestPath, err)
	}
	for root, paths := range manifest.Roots {
		state.roots[filepath.Clean(root)] = normalizeDirectorySyncPaths(paths)
	}
	return state, nil
}

// Sync 在同一个锁内对比、执行删除同步，并在成功后提交新的目录状态。
func (s *directorySyncState) Sync(root string, currentPaths []string, apply func(stalePaths []string) error) error {
	root = filepath.Clean(root)
	currentPaths = normalizeDirectorySyncPaths(currentPaths)

	s.mu.Lock()
	defer s.mu.Unlock()

	previousPaths := s.roots[root]
	stalePaths := diffDirectorySyncPaths(previousPaths, currentPaths)
	if err := apply(stalePaths); err != nil {
		return err
	}

	nextRoots := maps.Clone(s.roots)
	if len(currentPaths) == 0 {
		delete(nextRoots, root)
	} else {
		nextRoots[root] = slices.Clone(currentPaths)
	}
	if err := s.persist(nextRoots); err != nil {
		return err
	}
	s.roots = nextRoots
	return nil
}

func (s *directorySyncState) persist(roots map[string][]string) error {
	if s.manifestPath == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(s.manifestPath), 0o755); err != nil {
		return fmt.Errorf("mkdir directory sync manifest dir: %w", err)
	}

	payload := directorySyncManifest{
		Roots: roots,
	}
	raw, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal directory sync manifest: %w", err)
	}
	if err := os.WriteFile(s.manifestPath, raw, 0o600); err != nil {
		return fmt.Errorf("write directory sync manifest %q: %w", s.manifestPath, err)
	}
	return nil
}

func normalizeDirectorySyncPaths(paths []string) []string {
	if len(paths) == 0 {
		return nil
	}

	normalized := make([]string, 0, len(paths))
	for _, path := range paths {
		cleaned := strings.TrimSpace(path)
		if cleaned == "" {
			continue
		}
		normalized = append(normalized, filepath.Clean(cleaned))
	}
	if len(normalized) == 0 {
		return nil
	}

	slices.Sort(normalized)
	return slices.Compact(normalized)
}

func diffDirectorySyncPaths(previousPaths []string, currentPaths []string) []string {
	if len(previousPaths) == 0 {
		return nil
	}

	currentSet := make(map[string]struct{}, len(currentPaths))
	for _, path := range currentPaths {
		currentSet[path] = struct{}{}
	}

	stalePaths := make([]string, 0, len(previousPaths))
	for _, path := range previousPaths {
		if _, ok := currentSet[path]; ok {
			continue
		}
		stalePaths = append(stalePaths, path)
	}
	if len(stalePaths) == 0 {
		return nil
	}
	return stalePaths
}

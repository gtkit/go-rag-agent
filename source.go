package ragagent

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
)

type fileSource struct {
	path             string
	allowUnsupported bool
}

// FileSource 返回一个仅解析单个本地文本、HTML、图片或 PDF 文件的知识源。
func FileSource(path string) KnowledgeSource {
	return fileSource{path: path}
}

// ConvertibleFileSource 返回一个单文件知识源，允许通过 DocumentConverter 处理默认 loader 不支持的扩展名。
func ConvertibleFileSource(path string) KnowledgeSource {
	return fileSource{path: path, allowUnsupported: true}
}

func (s fileSource) Resolve(ctx context.Context) ([]KnowledgeFile, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !isSupportedKnowledgePath(s.path) && !s.allowUnsupported {
		return nil, fmt.Errorf("unsupported file extension for %q: %w", s.path, ErrUnsupportedSource)
	}
	info, err := os.Lstat(s.path)
	if err != nil {
		return nil, fmt.Errorf("stat source file %q: %w", s.path, err)
	}
	if info.IsDir() {
		return nil, fmt.Errorf("file source is directory %q: %w", s.path, ErrUnsupportedSource)
	}
	if info.Mode()&fs.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("file source must be a regular file %q: %w", s.path, ErrUnsupportedSource)
	}
	metadata, err := loadSidecarMetadataForSource(s.path)
	if err != nil {
		return nil, err
	}
	return []KnowledgeFile{
		{
			Path:     s.path,
			Title:    titleFromPath(s.path),
			Metadata: metadata,
		},
	}, nil
}

type dirSource struct {
	path string
}

// YoudaoNoteBridgeConfig 描述如何通过本地安装的 youdaonote 桥接命令导出笔记。
type YoudaoNoteBridgeConfig struct {
	Command string
	Args    []string
	Env     []string
	WorkDir string
}

type youdaoNoteSource struct {
	cfg       YoudaoNoteBridgeConfig
	mu        sync.Mutex
	exportDir string
}

// DirSource 返回一个递归解析目录内受支持文本、HTML、图片或 PDF 文件的知识源。
func DirSource(path string) KnowledgeSource {
	return dirSource{path: path}
}

// YoudaoNoteSource 返回一个有道笔记桥接知识源，通过本地 youdaonote 兼容命令导出数据。
// Args 中至少一个参数必须包含 "{output}" 占位符，运行时会替换成临时导出目录。
func YoudaoNoteSource(cfg YoudaoNoteBridgeConfig) KnowledgeSource {
	return &youdaoNoteSource{cfg: cfg}
}

func (s dirSource) Resolve(ctx context.Context) ([]KnowledgeFile, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	info, err := os.Lstat(s.path)
	if err != nil {
		return nil, fmt.Errorf("stat source dir %q: %w", s.path, err)
	}
	if info.Mode()&fs.ModeSymlink != 0 {
		return nil, fmt.Errorf("dir source must not be a symlink %q: %w", s.path, ErrUnsupportedSource)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("dir source is not a directory %q: %w", s.path, ErrUnsupportedSource)
	}

	files := make([]KnowledgeFile, 0)
	err = filepath.WalkDir(s.path, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if d.Type()&fs.ModeSymlink != 0 {
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		if isMetadataSidecarPath(path) {
			return nil
		}
		if !isSupportedKnowledgePath(path) {
			return nil
		}
		metadata, err := loadSidecarMetadataForSource(path)
		if err != nil {
			return err
		}
		files = append(files, KnowledgeFile{
			Path:     path,
			Title:    titleFromPath(path),
			Metadata: metadata,
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
	case ".txt", ".md", ".pdf", ".html", ".htm", ".png", ".jpg", ".jpeg", ".webp":
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

func (s *youdaoNoteSource) Resolve(ctx context.Context) ([]KnowledgeFile, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	command := strings.TrimSpace(s.cfg.Command)
	if command == "" {
		command = "youdaonote"
	}
	commandPath, err := exec.LookPath(command)
	if err != nil {
		return nil, fmt.Errorf("youdao bridge command %q not found: %w", command, ErrUnsupportedSource)
	}

	args, hasOutput := materializeBridgeArgs(s.cfg.Args, "")
	if len(args) == 0 || !hasOutput {
		return nil, fmt.Errorf("youdao bridge args must include {output}: %w", ErrUnsupportedSource)
	}

	exportDir, err := os.MkdirTemp("", "ragagent-youdao-*")
	if err != nil {
		return nil, fmt.Errorf("create youdao export dir: %w", err)
	}

	s.mu.Lock()
	if s.exportDir != "" {
		_ = os.RemoveAll(s.exportDir)
	}
	s.exportDir = exportDir
	s.mu.Unlock()

	args, _ = materializeBridgeArgs(s.cfg.Args, exportDir)
	cmd := exec.CommandContext(ctx, commandPath, args...)
	if s.cfg.WorkDir != "" {
		cmd.Dir = s.cfg.WorkDir
	}
	if len(s.cfg.Env) > 0 {
		cmd.Env = append(os.Environ(), s.cfg.Env...)
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("run youdao bridge export: %w: %s", err, strings.TrimSpace(string(out)))
	}

	files, err := dirSource{path: exportDir}.Resolve(ctx)
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("youdao bridge export produced no supported files: %w", ErrUnsupportedSource)
	}
	return files, nil
}

func (s *youdaoNoteSource) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.exportDir == "" {
		return nil
	}
	err := os.RemoveAll(s.exportDir)
	s.exportDir = ""
	return err
}

func materializeBridgeArgs(args []string, outputDir string) ([]string, bool) {
	resolved := make([]string, 0, len(args))
	hasOutput := false
	for _, arg := range args {
		if strings.Contains(arg, "{output}") {
			hasOutput = true
			arg = strings.ReplaceAll(arg, "{output}", outputDir)
		}
		resolved = append(resolved, arg)
	}
	return resolved, hasOutput
}

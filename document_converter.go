package ragagent

import (
	"context"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
)

// DocumentConverter converts an input file into a Document before the default loader runs.
type DocumentConverter interface {
	Name() string
	Supports(ctx context.Context, path string) (bool, error)
	Convert(ctx context.Context, path string, title string, metadata map[string]string) (Document, error)
}

// CommandDocumentConverterConfig configures a local command-backed document converter.
type CommandDocumentConverterConfig struct {
	Name       string
	Extensions []string
	Command    string
	Args       []string
	Env        []string
	WorkDir    string
	OutputExt  string
}

type commandDocumentConverter struct {
	cfg        CommandDocumentConverterConfig
	extensions []string
}

// NewCommandDocumentConverter creates a converter backed by a local command.
func NewCommandDocumentConverter(cfg CommandDocumentConverterConfig) (DocumentConverter, error) {
	cfg.Name = strings.TrimSpace(cfg.Name)
	if cfg.Name == "" {
		return nil, fmt.Errorf("document converter name is required: %w", ErrInvalidConfig)
	}
	cfg.Command = strings.TrimSpace(cfg.Command)
	if cfg.Command == "" {
		return nil, fmt.Errorf("document converter command is required: %w", ErrInvalidConfig)
	}
	if !argsContainPlaceholder(cfg.Args, "{input}") {
		return nil, fmt.Errorf("document converter args must contain {input}: %w", ErrInvalidConfig)
	}
	if !argsContainPlaceholder(cfg.Args, "{output}") {
		return nil, fmt.Errorf("document converter args must contain {output}: %w", ErrInvalidConfig)
	}
	extensions := normalizeExtensions(cfg.Extensions)
	if len(extensions) == 0 {
		return nil, fmt.Errorf("document converter extensions are required: %w", ErrInvalidConfig)
	}
	if strings.TrimSpace(cfg.OutputExt) == "" {
		cfg.OutputExt = ".md"
	}
	if !strings.HasPrefix(cfg.OutputExt, ".") {
		cfg.OutputExt = "." + cfg.OutputExt
	}
	return &commandDocumentConverter{cfg: cfg, extensions: extensions}, nil
}

func (c *commandDocumentConverter) Name() string { return c.cfg.Name }

func (c *commandDocumentConverter) Supports(_ context.Context, path string) (bool, error) {
	return slices.Contains(c.extensions, strings.ToLower(filepath.Ext(path))), nil
}

func (c *commandDocumentConverter) Convert(ctx context.Context, path string, title string, metadata map[string]string) (Document, error) {
	if err := ctx.Err(); err != nil {
		return Document{}, err
	}
	outputDir, err := os.MkdirTemp("", "ragagent-convert-*")
	if err != nil {
		return Document{}, fmt.Errorf("create converter temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(outputDir) }()

	outputPath := filepath.Join(outputDir, strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))+c.cfg.OutputExt)
	args := replaceCommandPlaceholders(c.cfg.Args, path, outputPath)
	cmd := exec.CommandContext(ctx, c.cfg.Command, args...)
	cmd.Env = append(os.Environ(), c.cfg.Env...)
	if strings.TrimSpace(c.cfg.WorkDir) != "" {
		cmd.Dir = c.cfg.WorkDir
	}
	if output, err := cmd.CombinedOutput(); err != nil {
		return Document{}, fmt.Errorf("convert document %q with %s: %w: %s", path, c.Name(), err, strings.TrimSpace(string(output)))
	}
	content, err := os.ReadFile(outputPath)
	if err != nil {
		return Document{}, fmt.Errorf("read converter output %q: %w", outputPath, err)
	}
	if len(strings.TrimSpace(string(content))) == 0 {
		return Document{}, fmt.Errorf("converter %s produced empty output for %q: %w", c.Name(), path, ErrUnsupportedSource)
	}
	return Document{
		ID:         path,
		SourcePath: path,
		Title:      title,
		Metadata:   maps.Clone(metadata),
		Content:    string(content),
	}, nil
}

func loadDocumentWithConverters(ctx context.Context, converters []DocumentConverter, loader DocumentLoader, path string, title string, metadata map[string]string, opts DocumentLoadOptions) (Document, error) {
	for _, converter := range converters {
		if converter == nil {
			continue
		}
		supported, err := converter.Supports(ctx, path)
		if err != nil {
			return Document{}, fmt.Errorf("check document converter %q for %q: %w", converter.Name(), path, err)
		}
		if !supported {
			continue
		}
		doc, err := converter.Convert(ctx, path, title, maps.Clone(metadata))
		if err != nil {
			return Document{}, fmt.Errorf("convert knowledge file %q with %s: %w", path, converter.Name(), err)
		}
		if strings.TrimSpace(doc.SourcePath) == "" {
			doc.SourcePath = path
		}
		if strings.TrimSpace(doc.Title) == "" {
			doc.Title = title
		}
		if doc.Metadata == nil {
			doc.Metadata = maps.Clone(metadata)
		}
		return doc, nil
	}
	return loader.Load(ctx, path, title, metadata, opts)
}

func normalizeExtensions(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, 0, len(in))
	for _, ext := range in {
		ext = strings.ToLower(strings.TrimSpace(ext))
		if ext == "" {
			continue
		}
		if !strings.HasPrefix(ext, ".") {
			ext = "." + ext
		}
		out = append(out, ext)
	}
	if len(out) == 0 {
		return nil
	}
	slices.Sort(out)
	return slices.Compact(out)
}

func argsContainPlaceholder(args []string, placeholder string) bool {
	for _, arg := range args {
		if strings.Contains(arg, placeholder) {
			return true
		}
	}
	return false
}

func replaceCommandPlaceholders(args []string, input string, output string) []string {
	out := make([]string, 0, len(args))
	for _, arg := range args {
		arg = strings.ReplaceAll(arg, "{input}", input)
		arg = strings.ReplaceAll(arg, "{output}", output)
		out = append(out, arg)
	}
	return out
}

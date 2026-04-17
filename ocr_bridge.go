package ragagent

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
)

// PDFOCRBridgeConfig 描述扫描版 PDF 的本地 OCR bridge 配置。
type PDFOCRBridgeConfig struct {
	Command            string
	Args               []string
	Env                []string
	WorkDir            string
	MinDirectTextRunes int
}

func (c PDFOCRBridgeConfig) normalized() PDFOCRBridgeConfig {
	c.Command = strings.TrimSpace(c.Command)
	c.Args = slices.Clone(c.Args)
	c.Env = slices.Clone(c.Env)
	c.WorkDir = strings.TrimSpace(c.WorkDir)
	return c
}

func (c PDFOCRBridgeConfig) enabled() bool {
	return c.Command != "" || len(c.Args) > 0 || len(c.Env) > 0 || c.WorkDir != "" || c.MinDirectTextRunes != 0
}

func (c PDFOCRBridgeConfig) validate() error {
	c = c.normalized()
	if !c.enabled() {
		return nil
	}
	if c.Command == "" {
		return fmt.Errorf("pdf ocr bridge command is required: %w", ErrInvalidConfig)
	}
	if c.MinDirectTextRunes < 0 {
		return fmt.Errorf("pdf ocr bridge min direct text runes must be >= 0: %w", ErrInvalidConfig)
	}

	_, hasInput, hasOutput := materializePDFOCRBridgeArgs(c.Args, "", "")
	if !hasInput || !hasOutput {
		return fmt.Errorf("pdf ocr bridge args must include {input} and {output}: %w", ErrInvalidConfig)
	}
	return nil
}

func (c PDFOCRBridgeConfig) extractor() func(context.Context, string) (string, error) {
	c = c.normalized()
	if !c.enabled() {
		return nil
	}
	return func(ctx context.Context, inputPath string) (string, error) {
		return c.extractText(ctx, inputPath)
	}
}

func (c PDFOCRBridgeConfig) extractText(ctx context.Context, inputPath string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}

	commandPath, err := exec.LookPath(c.Command)
	if err != nil {
		return "", fmt.Errorf("pdf ocr bridge command %q not found: %w", c.Command, err)
	}

	tempDir, err := os.MkdirTemp("", "ragagent-pdf-ocr-*")
	if err != nil {
		return "", fmt.Errorf("create pdf ocr temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	outputPath := filepath.Join(tempDir, "output.txt")
	args, _, _ := materializePDFOCRBridgeArgs(c.Args, inputPath, outputPath)
	cmd := exec.CommandContext(ctx, commandPath, args...)
	if c.WorkDir != "" {
		cmd.Dir = c.WorkDir
	}
	if len(c.Env) > 0 {
		cmd.Env = append(os.Environ(), c.Env...)
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("run pdf ocr bridge: %w: %s", err, strings.TrimSpace(string(out)))
	}

	data, err := os.ReadFile(outputPath)
	if err != nil {
		return "", fmt.Errorf("read pdf ocr output %q: %w", outputPath, err)
	}
	text := strings.TrimSpace(string(data))
	if text == "" {
		return "", fmt.Errorf("pdf ocr bridge produced empty text for %q", inputPath)
	}
	return text, nil
}

func materializePDFOCRBridgeArgs(args []string, inputPath string, outputPath string) ([]string, bool, bool) {
	resolved := make([]string, 0, len(args))
	hasInput := false
	hasOutput := false
	for _, arg := range args {
		if strings.Contains(arg, "{input}") {
			hasInput = true
			arg = strings.ReplaceAll(arg, "{input}", inputPath)
		}
		if strings.Contains(arg, "{output}") {
			hasOutput = true
			arg = strings.ReplaceAll(arg, "{output}", outputPath)
		}
		resolved = append(resolved, arg)
	}
	return resolved, hasInput, hasOutput
}

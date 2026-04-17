package rag

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

func extractMarkdownFrontMatter(content string) (string, map[string]string, error) {
	normalized := strings.ReplaceAll(content, "\r\n", "\n")
	if !strings.HasPrefix(normalized, "---\n") {
		return content, map[string]string{}, nil
	}

	rest := normalized[len("---\n"):]
	endIndex := strings.Index(rest, "\n---\n")
	endMarkerLen := len("\n---\n")
	if endIndex < 0 {
		endIndex = strings.Index(rest, "\n...\n")
		endMarkerLen = len("\n...\n")
	}
	if endIndex < 0 {
		return "", nil, fmt.Errorf("unterminated markdown front matter")
	}

	rawMetadata := rest[:endIndex]
	body := strings.TrimLeft(rest[endIndex+endMarkerLen:], "\n")

	metadata, err := parseFrontMatterMetadata(rawMetadata)
	if err != nil {
		return "", nil, err
	}
	return body, metadata, nil
}

func parseFrontMatterMetadata(raw string) (map[string]string, error) {
	if strings.TrimSpace(raw) == "" {
		return map[string]string{}, nil
	}

	var metadata map[string]any
	if err := yaml.Unmarshal([]byte(raw), &metadata); err != nil {
		return nil, fmt.Errorf("parse markdown front matter: %w", err)
	}
	return stringifyMetadataMap(metadata)
}

package ragagent

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

var sidecarMetadataExtensions = []string{".meta.yaml", ".meta.yml", ".meta.json"}

func loadSidecarMetadataForSource(path string) (map[string]string, error) {
	for _, sidecarPath := range candidateSidecarMetadataPaths(path) {
		metadata, found, err := loadMetadataMapFile(sidecarPath)
		if err != nil {
			return nil, fmt.Errorf("load sidecar metadata %q: %w", sidecarPath, err)
		}
		if found {
			return metadata, nil
		}
	}
	return map[string]string{}, nil
}

func candidateSidecarMetadataPaths(path string) []string {
	ext := filepath.Ext(path)
	base := strings.TrimSuffix(path, ext)
	paths := make([]string, 0, len(sidecarMetadataExtensions))
	for _, suffix := range sidecarMetadataExtensions {
		paths = append(paths, base+suffix)
	}
	return paths
}

func isMetadataSidecarPath(path string) bool {
	lower := strings.ToLower(path)
	for _, suffix := range sidecarMetadataExtensions {
		if strings.HasSuffix(lower, suffix) {
			return true
		}
	}
	return false
}

func loadMetadataMapFile(path string) (map[string]string, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, false, nil
		}
		return nil, false, err
	}

	metadata, err := parseMetadataMap(data, filepath.Ext(path))
	if err != nil {
		return nil, false, err
	}
	return metadata, true, nil
}

func parseMetadataMap(data []byte, ext string) (map[string]string, error) {
	var raw map[string]any
	switch strings.ToLower(ext) {
	case ".json":
		if err := json.Unmarshal(data, &raw); err != nil {
			return nil, fmt.Errorf("parse json metadata: %w", err)
		}
	case ".yaml", ".yml":
		if err := yaml.Unmarshal(data, &raw); err != nil {
			return nil, fmt.Errorf("parse yaml metadata: %w", err)
		}
	default:
		return nil, fmt.Errorf("unsupported metadata file extension %q", ext)
	}
	return stringifyMetadataMap(raw)
}

func stringifyMetadataMap(raw map[string]any) (map[string]string, error) {
	if len(raw) == 0 {
		return map[string]string{}, nil
	}

	metadata := make(map[string]string, len(raw))
	for key, value := range raw {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		stringValue, err := stringifyMetadataValue(value)
		if err != nil {
			return nil, fmt.Errorf("stringify metadata key %q: %w", key, err)
		}
		metadata[key] = stringValue
	}
	return metadata, nil
}

func stringifyMetadataValue(value any) (string, error) {
	switch v := value.(type) {
	case nil:
		return "", nil
	case string:
		return v, nil
	case bool:
		if v {
			return "true", nil
		}
		return "false", nil
	case int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64,
		float32, float64:
		return fmt.Sprint(v), nil
	default:
		data, err := json.Marshal(v)
		if err != nil {
			return "", err
		}
		return string(data), nil
	}
}

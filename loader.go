package tune

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// loadYAMLConfig loads YAML configuration from various sources.
// It supports three modes:
//  1. path == "" — returns an empty config (ENV variables only)
//  2. path points to a file — loads a single YAML file
//  3. path points to a directory — merges all YAML files from the directory
//
// Returns a YAML Node (AST), a map of ModTime per file, and an error.
// Using yaml.Node instead of map[string]any avoids double marshaling when decoding sections.
func loadYAMLConfig(path string) (*yaml.Node, map[string]time.Time, error) {
	if path == "" {
		return &yaml.Node{Kind: yaml.MappingNode}, make(map[string]time.Time), nil
	}

	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &yaml.Node{Kind: yaml.MappingNode}, make(map[string]time.Time), nil
		}
		return nil, nil, fmt.Errorf("failed to stat config path %q: %w", path, err)
	}

	if !info.IsDir() {
		return loadSingleYAMLFile(path, info)
	}

	return loadAndMergeYAML(path)
}

// loadSingleYAMLFile loads a single YAML file and returns a yaml.Node (AST).
func loadSingleYAMLFile(filePath string, info os.FileInfo) (*yaml.Node, map[string]time.Time, error) {
	ext := strings.ToLower(filepath.Ext(filePath))
	if ext != ".yml" && ext != ".yaml" {
		return nil, nil, fmt.Errorf("config file must have .yml or .yaml extension, got %q", filePath)
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read config file %q: %w", filePath, err)
	}

	var node yaml.Node
	if err := yaml.Unmarshal(data, &node); err != nil {
		return nil, nil, fmt.Errorf("failed to parse YAML file %q: %w", filePath, err)
	}

	fileName := filepath.Base(filePath)
	fileModTimes := map[string]time.Time{
		fileName: info.ModTime(),
	}

	return &node, fileModTimes, nil
}

// loadAndMergeYAML reads all .yml and .yaml files from the given directory
// and merges their contents into a single yaml.Node.
// Returns the merged config as a Node, a map of ModTime per file, and an error.
//
// When multiple files contain the same top-level key, values from later files
// overwrite earlier ones.
func loadAndMergeYAML(dir string) (*yaml.Node, map[string]time.Time, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read config directory: %w", err)
	}

	merged := make(map[string]any)
	fileModTimes := make(map[string]time.Time)

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		ext := strings.ToLower(filepath.Ext(name))
		if ext != ".yml" && ext != ".yaml" {
			continue
		}

		path := filepath.Join(dir, name)
		info, err := entry.Info()
		if err != nil {
			return nil, nil, fmt.Errorf("failed to get file info for %q: %w", name, err)
		}

		fileModTimes[name] = info.ModTime()

		data, err := os.ReadFile(path)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to read file %q: %w", name, err)
		}

		var fileConfig map[string]any
		if err := yaml.Unmarshal(data, &fileConfig); err != nil {
			return nil, nil, fmt.Errorf("failed to parse YAML file %q: %w", name, err)
		}

		for k, v := range fileConfig {
			merged[k] = v
		}
	}

	// Convert the merged map directly into a yaml.Node (AST) to avoid
	// a redundant round-trip through YAML bytes in decodeSection.
	node := &yaml.Node{}
	if err := node.Encode(merged); err != nil {
		return nil, nil, fmt.Errorf("failed to encode merged config into YAML node: %w", err)
	}

	return node, fileModTimes, nil
}

// decodeSection decodes section data from a yaml.Node into a Section struct.
// It decodes directly from the AST, avoiding double marshaling (map -> yaml -> struct).
func decodeSection(rootNode *yaml.Node, sectionKey string, section Section) error {
	sectionNode := findSectionNode(rootNode, sectionKey)
	if sectionNode == nil {
		return nil
	}

	// Direct decode from the AST node into the target struct.
	// Strict mode (rejecting unknown fields) would require serializing the node
	// first, so it is omitted here in favor of direct decoding performance.
	if err := sectionNode.Decode(section); err != nil {
		return fmt.Errorf("failed to decode section %q: %w", sectionKey, err)
	}

	return nil
}

// findSectionNode locates a section node by key within a root mapping node.
func findSectionNode(rootNode *yaml.Node, key string) *yaml.Node {
	// yaml.Unmarshal produces a Document node wrapping a Mapping node;
	// we need to unwrap to the actual mapping.
	var mappingNode *yaml.Node
	switch {
	case rootNode.Kind == yaml.DocumentNode && len(rootNode.Content) > 0:
		mappingNode = rootNode.Content[0]
	case rootNode.Kind == yaml.MappingNode:
		mappingNode = rootNode
	default:
		return nil
	}

	// Mapping node stores interleaved key-value pairs in Content:
	// Content[0]=key1, Content[1]=value1, Content[2]=key2, Content[3]=value2, ...
	for i := 0; i < len(mappingNode.Content); i += 2 {
		if i+1 >= len(mappingNode.Content) {
			break
		}
		keyNode := mappingNode.Content[i]
		valueNode := mappingNode.Content[i+1]

		if keyNode.Value == key {
			return valueNode
		}
	}

	return nil
}

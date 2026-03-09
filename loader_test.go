package tune

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

// nodeToMap is a test helper that converts a yaml.Node back into map[string]any.
func nodeToMap(node *yaml.Node) (map[string]any, error) {
	var result map[string]any
	if err := node.Decode(&result); err != nil {
		return nil, err
	}
	return result, nil
}

func TestLoadAndMergeYAML_SingleFile(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	yamlContent := `server:
  port: 8080
  host: "localhost"
`
	configFile := filepath.Join(tmpDir, "config.yml")
	if err := os.WriteFile(configFile, []byte(yamlContent), 0o644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	merged, fileModTimes, err := loadAndMergeYAML(tmpDir)
	if err != nil {
		t.Fatalf("loadAndMergeYAML() failed: %v", err)
	}

	if len(fileModTimes) == 0 {
		t.Error("expected fileModTimes to be populated")
	}

	mergedMap, err := nodeToMap(merged)
	if err != nil {
		t.Fatalf("failed to convert node to map: %v", err)
	}

	server, ok := mergedMap["server"].(map[string]any)
	if !ok {
		t.Fatal("server section not found or wrong type")
	}

	if server["port"].(int) != 8080 {
		t.Errorf("expected port 8080, got %v", server["port"])
	}

	if server["host"].(string) != "localhost" {
		t.Errorf("expected host localhost, got %v", server["host"])
	}
}

func TestLoadAndMergeYAML_MultipleFiles(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	file1 := filepath.Join(tmpDir, "server.yml")
	file1Content := `server:
  port: 8080
  host: "localhost"
`
	if err := os.WriteFile(file1, []byte(file1Content), 0o644); err != nil {
		t.Fatalf("failed to write file1: %v", err)
	}

	file2 := filepath.Join(tmpDir, "database.yml")
	file2Content := `database:
  host: "db.example.com"
  port: 5432
`
	if err := os.WriteFile(file2, []byte(file2Content), 0o644); err != nil {
		t.Fatalf("failed to write file2: %v", err)
	}

	merged, _, err := loadAndMergeYAML(tmpDir)
	if err != nil {
		t.Fatalf("loadAndMergeYAML() failed: %v", err)
	}

	mergedMap, err := nodeToMap(merged)
	if err != nil {
		t.Fatalf("failed to convert node to map: %v", err)
	}

	server, ok := mergedMap["server"].(map[string]any)
	if !ok {
		t.Fatal("server section not found")
	}
	if server["port"].(int) != 8080 {
		t.Errorf("expected port 8080, got %v", server["port"])
	}
	if server["host"].(string) != "localhost" {
		t.Errorf("expected host localhost, got %v", server["host"])
	}

	database, ok := mergedMap["database"].(map[string]any)
	if !ok {
		t.Fatal("database section not found")
	}
	if database["port"].(int) != 5432 {
		t.Errorf("expected database port 5432, got %v", database["port"])
	}
}

func TestLoadAndMergeYAML_IgnoreNonYAML(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	yamlFile := filepath.Join(tmpDir, "config.yml")
	if err := os.WriteFile(yamlFile, []byte("server:\n  port: 8080\n"), 0o644); err != nil {
		t.Fatalf("failed to write yaml: %v", err)
	}

	// Non-YAML file (must be ignored).
	txtFile := filepath.Join(tmpDir, "readme.txt")
	if err := os.WriteFile(txtFile, []byte("This is not YAML"), 0o644); err != nil {
		t.Fatalf("failed to write txt: %v", err)
	}

	merged, _, err := loadAndMergeYAML(tmpDir)
	if err != nil {
		t.Fatalf("loadAndMergeYAML() failed: %v", err)
	}

	mergedMap, err := nodeToMap(merged)
	if err != nil {
		t.Fatalf("failed to convert node to map: %v", err)
	}

	// Only the server section should be present.
	if _, ok := mergedMap["server"]; !ok {
		t.Error("server section not found")
	}
	if len(mergedMap) != 1 {
		t.Errorf("expected 1 section, got %d", len(mergedMap))
	}
}

func TestLoadAndMergeYAML_CaseInsensitiveExtension(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	// Files with mixed-case extensions.
	file1 := filepath.Join(tmpDir, "config.YML")
	if err := os.WriteFile(file1, []byte("server:\n  port: 8080\n"), 0o644); err != nil {
		t.Fatalf("failed to write file1: %v", err)
	}

	file2 := filepath.Join(tmpDir, "config.YAML")
	if err := os.WriteFile(file2, []byte("database:\n  port: 5432\n"), 0o644); err != nil {
		t.Fatalf("failed to write file2: %v", err)
	}

	file3 := filepath.Join(tmpDir, "config.yml")
	if err := os.WriteFile(file3, []byte("logger:\n  level: info\n"), 0o644); err != nil {
		t.Fatalf("failed to write file3: %v", err)
	}

	merged, _, err := loadAndMergeYAML(tmpDir)
	if err != nil {
		t.Fatalf("loadAndMergeYAML() failed: %v", err)
	}

	mergedMap, err := nodeToMap(merged)
	if err != nil {
		t.Fatalf("failed to convert node to map: %v", err)
	}

	// All three files should be read regardless of extension case.
	if len(mergedMap) != 3 {
		t.Errorf("expected 3 sections, got %d", len(mergedMap))
	}
}

func TestLoadAndMergeYAML_EmptyDirectory(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	merged, fileModTimes, err := loadAndMergeYAML(tmpDir)
	if err != nil {
		t.Fatalf("loadAndMergeYAML() failed: %v", err)
	}

	mergedMap, err := nodeToMap(merged)
	if err != nil {
		t.Fatalf("failed to convert node to map: %v", err)
	}

	if len(mergedMap) != 0 {
		t.Errorf("expected empty map, got %d sections", len(mergedMap))
	}

	// fileModTimes must be empty for an empty directory.
	if len(fileModTimes) != 0 {
		t.Error("expected empty fileModTimes for empty directory")
	}
}

func TestLoadAndMergeYAML_InvalidYAML(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	invalidFile := filepath.Join(tmpDir, "invalid.yml")
	invalidContent := `server:
  port: [unclosed bracket
`
	if err := os.WriteFile(invalidFile, []byte(invalidContent), 0o644); err != nil {
		t.Fatalf("failed to write invalid file: %v", err)
	}

	_, _, err := loadAndMergeYAML(tmpDir)
	if err == nil {
		t.Fatal("expected error for invalid YAML, got nil")
	}
}

func TestLoadAndMergeYAML_ModTime(t *testing.T) {
	tmpDir := t.TempDir()

	file1 := filepath.Join(tmpDir, "config1.yml")
	if err := os.WriteFile(file1, []byte("server:\n  port: 8080\n"), 0o644); err != nil {
		t.Fatalf("failed to write file1: %v", err)
	}

	// Ensure distinct ModTime values between files.
	time.Sleep(10 * time.Millisecond)

	file2 := filepath.Join(tmpDir, "config2.yml")
	if err := os.WriteFile(file2, []byte("database:\n  port: 5432\n"), 0o644); err != nil {
		t.Fatalf("failed to write file2: %v", err)
	}

	_, fileModTimes, err := loadAndMergeYAML(tmpDir)
	if err != nil {
		t.Fatalf("loadAndMergeYAML() failed: %v", err)
	}

	// fileModTimes must contain entries for both files.
	if len(fileModTimes) != 2 {
		t.Errorf("expected 2 files in fileModTimes, got %d", len(fileModTimes))
	}

	// Verify that recorded mod times match actual file timestamps.
	info1, err := os.Stat(file1)
	if err != nil {
		t.Fatalf("Stat file1 failed: %v", err)
	}
	info2, err := os.Stat(file2)
	if err != nil {
		t.Fatalf("Stat file2 failed: %v", err)
	}

	if mt, exists := fileModTimes["config1.yml"]; !exists {
		t.Error("config1.yml not found in fileModTimes")
	} else if !mt.Equal(info1.ModTime()) {
		t.Errorf("config1.yml modTime mismatch: got %v, expected %v", mt, info1.ModTime())
	}

	if mt, exists := fileModTimes["config2.yml"]; !exists {
		t.Error("config2.yml not found in fileModTimes")
	} else if !mt.Equal(info2.ModTime()) {
		t.Errorf("config2.yml modTime mismatch: got %v, expected %v", mt, info2.ModTime())
	}
}

func TestDecodeSection(t *testing.T) {
	t.Parallel()
	sectionData := map[string]any{
		"port":    8080,
		"host":    "localhost",
		"timeout": "5s",
	}

	rootNode := &yaml.Node{}
	if err := rootNode.Encode(map[string]any{"test": sectionData}); err != nil {
		t.Fatalf("failed to encode test data: %v", err)
	}

	section := &testSection{}
	if err := decodeSection(rootNode, "test", section); err != nil {
		t.Fatalf("decodeSection() failed: %v", err)
	}

	if section.Port != 8080 {
		t.Errorf("expected port 8080, got %d", section.Port)
	}
	if section.Host != "localhost" {
		t.Errorf("expected host localhost, got %q", section.Host)
	}
}

func TestDecodeSection_InvalidData(t *testing.T) {
	t.Parallel()
	// Invalid yaml.Node: scalar string instead of a mapping.
	rootNode := &yaml.Node{}
	if err := rootNode.Encode(map[string]any{"test": "not a struct"}); err != nil {
		t.Fatalf("failed to encode test data: %v", err)
	}

	section := &testSection{}
	err := decodeSection(rootNode, "test", section)
	if err == nil {
		t.Fatal("expected error for invalid section data, got nil")
	}
}

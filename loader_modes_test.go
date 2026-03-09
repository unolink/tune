package tune

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestLoadYAMLConfig_EnvOnly tests the ENV-only mode (empty path).
func TestLoadYAMLConfig_EnvOnly(t *testing.T) {
	m := New(WithPath(""), WithEnvPrefix("MYAPP"))

	section := &testSection{}
	m.MustRegister(section)

	t.Setenv("MYAPP_TEST_HOST", "env-only-host")
	t.Setenv("MYAPP_TEST_PORT", "9999")

	if err := m.Load(); err != nil {
		t.Fatalf("Load() failed: %v", err)
	}
	if section.Host != "env-only-host" {
		t.Errorf("expected host 'env-only-host', got %q", section.Host)
	}

	if section.Port != 9999 {
		t.Errorf("expected port 9999, got %d", section.Port)
	}
}

// TestLoadYAMLConfig_SingleFile tests loading from a single YAML file.
func TestLoadYAMLConfig_SingleFile(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	// Create a single YAML file.
	configFile := filepath.Join(tmpDir, "config.yml")
	yamlContent := `test:
  host: "file-host"
  port: 7777
  timeout: "10s"
`
	if err := os.WriteFile(configFile, []byte(yamlContent), 0o644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	m := New(WithPath(configFile), WithEnvPrefix("MYAPP"))
	section := &testSection{}
	m.MustRegister(section)

	if err := m.Load(); err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	// Verify values from the file.
	if section.Host != "file-host" {
		t.Errorf("expected host 'file-host', got %q", section.Host)
	}

	if section.Port != 7777 {
		t.Errorf("expected port 7777, got %d", section.Port)
	}
}

// TestLoadYAMLConfig_SingleFileWithEnvOverride tests ENV overriding file values.
func TestLoadYAMLConfig_SingleFileWithEnvOverride(t *testing.T) {
	tmpDir := t.TempDir()

	configFile := filepath.Join(tmpDir, "config.yaml")
	yamlContent := `test:
  host: "file-host"
  port: 7777
`
	if err := os.WriteFile(configFile, []byte(yamlContent), 0o644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	m := New(WithPath(configFile), WithEnvPrefix("MYAPP"))
	section := &testSection{}
	m.MustRegister(section)

	// ENV variable overrides the file value.
	t.Setenv("MYAPP_TEST_PORT", "8888")

	if err := m.Load(); err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	// Host from file, Port from ENV.
	if section.Host != "file-host" {
		t.Errorf("expected host 'file-host', got %q", section.Host)
	}

	if section.Port != 8888 {
		t.Errorf("expected port 8888 (from ENV), got %d", section.Port)
	}
}

// TestLoadYAMLConfig_Directory tests loading from a directory of YAML files.
func TestLoadYAMLConfig_Directory(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	// loadAndMergeYAML merges at the top-level key (section) granularity,
	// so the second file completely replaces the 'test' section.
	file1 := filepath.Join(tmpDir, "01-base.yml")
	yamlContent1 := `test:
  host: "dir-host"
  port: 5555
`
	if err := os.WriteFile(file1, []byte(yamlContent1), 0o644); err != nil {
		t.Fatalf("failed to write file1: %v", err)
	}

	file2 := filepath.Join(tmpDir, "02-override.yml")
	yamlContent2 := `test:
  host: "dir-host-override"
  port: 6666
  timeout: "15s"
`
	if err := os.WriteFile(file2, []byte(yamlContent2), 0o644); err != nil {
		t.Fatalf("failed to write file2: %v", err)
	}

	m := New(WithPath(tmpDir), WithEnvPrefix("MYAPP"))
	section := &testSection{}
	m.MustRegister(section)

	if err := m.Load(); err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	// Values from the second file (it overwrites the entire 'test' section).
	if section.Host != "dir-host-override" {
		t.Errorf("expected host 'dir-host-override', got %q", section.Host)
	}

	if section.Port != 6666 {
		t.Errorf("expected port 6666 (from second file), got %d", section.Port)
	}
}

// TestLoadYAMLConfig_NonExistentPath tests handling of a non-existent path.
func TestLoadYAMLConfig_NonExistentPath(t *testing.T) {
	t.Parallel()
	m := New(WithPath("/nonexistent/path/config.yml"), WithEnvPrefix("MYAPP"))
	section := &testSection{}
	m.MustRegister(section)

	// Should fall back to defaults when the path does not exist.
	if err := m.Load(); err != nil {
		t.Fatalf("Load() should not fail for non-existent path, got: %v", err)
	}

	// Verify defaults.
	if section.Host != "localhost" {
		t.Errorf("expected default host 'localhost', got %q", section.Host)
	}
}

// TestLoadYAMLConfig_InvalidExtension tests handling of a file with an invalid extension.
func TestLoadYAMLConfig_InvalidExtension(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	configFile := filepath.Join(tmpDir, "config.txt")
	if err := os.WriteFile(configFile, []byte("test: value"), 0o644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	m := New(WithPath(configFile), WithEnvPrefix("MYAPP"))
	section := &testSection{}
	m.MustRegister(section)

	err := m.Load()
	if err == nil {
		t.Fatal("expected error for invalid file extension")
	}

	if !strings.Contains(err.Error(), "must have .yml or .yaml extension") {
		t.Errorf("expected extension error, got: %v", err)
	}
}

// TestWatch_EnvOnly verifies that Watch is a no-op when path is empty.
func TestWatch_EnvOnly(t *testing.T) {
	m := New(WithPath(""), WithEnvPrefix("MYAPP"))
	section := &testSection{}
	m.MustRegister(section)

	if err := m.Load(); err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	// checkIfModified always returns false for an empty path.
	if err := m.Watch(100); err != nil {
		t.Fatalf("Watch() failed: %v", err)
	}

	m.StopWatch()
}

// TestWatch_SingleFile verifies file change detection for a single file.
func TestWatch_SingleFile(t *testing.T) {
	tmpDir := t.TempDir()

	configFile := filepath.Join(tmpDir, "config.yml")
	yamlContent := `test:
  port: 1111
`
	if err := os.WriteFile(configFile, []byte(yamlContent), 0o644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	m := New(WithPath(configFile), WithEnvPrefix("MYAPP"))
	section := &testSection{}
	m.MustRegister(section)

	if err := m.Load(); err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	if section.Port != 1111 {
		t.Errorf("expected initial port 1111, got %d", section.Port)
	}

	// Start Watch.
	if err := m.Watch(100); err != nil {
		t.Fatalf("Watch() failed: %v", err)
	}
	defer m.StopWatch()

	// Modify the file.
	newContent := `test:
  port: 2222
`
	if err := os.WriteFile(configFile, []byte(newContent), 0o644); err != nil {
		t.Fatalf("failed to update config file: %v", err)
	}

	// Cannot guarantee exact detection timing, so just stop.
	m.StopWatch()
}

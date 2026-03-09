package tune

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// testSection is a test config section for functional verification.
type testSection struct {
	Host    string        `yaml:"host"`
	Port    int           `yaml:"port"`
	Timeout time.Duration `yaml:"timeout"`
	Enabled bool          `yaml:"enabled"`
}

func (t *testSection) ConfigKey() string { return "test" }
func (t *testSection) SetDefaults() {
	t.Port = 8080
	t.Host = "localhost"
	t.Timeout = 5 * time.Second
	t.Enabled = true
}
func (t *testSection) Validate() error {
	if t.Port <= 0 {
		return &ValidationError{Section: "test", Message: "port must be positive"}
	}
	return nil
}
func (t *testSection) OnUpdate() {
	// no-op callback for testing
}

// ValidationError is a test validation error.
type ValidationError struct {
	Section string
	Message string
}

func (e *ValidationError) Error() string {
	return e.Message
}

func TestManager_Register(t *testing.T) {
	t.Parallel()
	m := New(WithPath("/tmp"), WithEnvPrefix("TEST"))

	section := &testSection{}
	m.MustRegister(section)

	m.mu.RLock()
	if len(m.sections) != 1 {
		t.Errorf("expected 1 section, got %d", len(m.sections))
	}
	if m.sections["test"] != section {
		t.Error("section not registered correctly")
	}
	m.mu.RUnlock()
}

func TestManager_Register_PanicOnNil(t *testing.T) {
	t.Parallel()
	m := New(WithPath("/tmp"), WithEnvPrefix("TEST"))

	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic on nil section")
		}
	}()

	m.MustRegister(nil)
}

func TestManager_Register_PanicOnEmptyKey(t *testing.T) {
	t.Parallel()
	m := New(WithPath("/tmp"), WithEnvPrefix("TEST"))

	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic on empty ConfigKey")
		}
	}()

	section := &emptyKeySection{}
	m.MustRegister(section)
}

type emptyKeySection struct{}

func (e *emptyKeySection) ConfigKey() string { return "" }
func (e *emptyKeySection) SetDefaults()      {}
func (e *emptyKeySection) Validate() error   { return nil }
func (e *emptyKeySection) OnUpdate()         {}

func TestManager_Load_WithYAML(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	yamlContent := `test:
  port: 9000
  host: "0.0.0.0"
  timeout: "10s"
  enabled: false
`
	configFile := filepath.Join(tmpDir, "config.yml")
	if err := os.WriteFile(configFile, []byte(yamlContent), 0o644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	m := New(WithPath(tmpDir), WithEnvPrefix("TEST"))
	section := &testSection{}
	m.MustRegister(section)

	if err := m.Load(); err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	if section.Port != 9000 {
		t.Errorf("expected port 9000, got %d", section.Port)
	}
	if section.Host != "0.0.0.0" {
		t.Errorf("expected host 0.0.0.0, got %q", section.Host)
	}
	if section.Timeout != 10*time.Second {
		t.Errorf("expected timeout 10s, got %v", section.Timeout)
	}
	if section.Enabled != false {
		t.Errorf("expected enabled false, got %v", section.Enabled)
	}
}

func TestManager_Load_WithDefaults(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	m := New(WithPath(tmpDir), WithEnvPrefix("TEST"))
	section := &testSection{}
	m.MustRegister(section)

	// Without YAML files, defaults should be used.
	if err := m.Load(); err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	if section.Port != 8080 {
		t.Errorf("expected default port 8080, got %d", section.Port)
	}
	if section.Host != "localhost" {
		t.Errorf("expected default host localhost, got %q", section.Host)
	}
}

func TestManager_Load_WithENV(t *testing.T) {
	tmpDir := t.TempDir()

	t.Setenv("TEST_TEST_PORT", "7777")
	t.Setenv("TEST_TEST_HOST", "env-host")
	t.Setenv("TEST_TEST_TIMEOUT", "30s")
	t.Setenv("TEST_TEST_ENABLED", "false")

	yamlContent := `test:
  port: 9000
  host: "yaml-host"
`
	configFile := filepath.Join(tmpDir, "config.yml")
	if err := os.WriteFile(configFile, []byte(yamlContent), 0o644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	m := New(WithPath(tmpDir), WithEnvPrefix("TEST"))
	section := &testSection{}
	m.MustRegister(section)

	if err := m.Load(); err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	// ENV must override YAML values.
	if section.Port != 7777 {
		t.Errorf("expected port 7777 from ENV, got %d", section.Port)
	}
	if section.Host != "env-host" {
		t.Errorf("expected host env-host from ENV, got %q", section.Host)
	}
	if section.Timeout != 30*time.Second {
		t.Errorf("expected timeout 30s from ENV, got %v", section.Timeout)
	}
	if section.Enabled != false {
		t.Errorf("expected enabled false from ENV, got %v", section.Enabled)
	}
}

func TestManager_Load_ValidationError(t *testing.T) {
	tmpDir := t.TempDir()

	t.Setenv("TEST_TEST_PORT", "0")

	m := New(WithPath(tmpDir), WithEnvPrefix("TEST"))
	section := &testSection{}
	m.MustRegister(section)

	err := m.Load()
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}

	if !strings.Contains(err.Error(), "validation failed") {
		t.Errorf("expected validation error message, got: %v", err)
	}
}

func TestManager_Load_MergeMultipleFiles(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	file1 := filepath.Join(tmpDir, "server.yml")
	file1Content := `test:
  port: 9000
  host: "0.0.0.0"
`
	if err := os.WriteFile(file1, []byte(file1Content), 0o644); err != nil {
		t.Fatalf("failed to write file1: %v", err)
	}

	file2 := filepath.Join(tmpDir, "timeout.yml")
	file2Content := `test:
  timeout: "20s"
  enabled: true
`
	if err := os.WriteFile(file2, []byte(file2Content), 0o644); err != nil {
		t.Fatalf("failed to write file2: %v", err)
	}

	m := New(WithPath(tmpDir), WithEnvPrefix("TEST"))
	section := &testSection{}
	m.MustRegister(section)

	if err := m.Load(); err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	// File read order is non-deterministic, so we only verify that all fields are populated.
	if section.Port == 0 {
		t.Error("expected port to be set, got 0")
	}
	if section.Host == "" {
		t.Error("expected host to be set, got empty string")
	}
	if section.Timeout == 0 {
		t.Error("expected timeout to be set, got 0")
	}
	if section.Port != 9000 && section.Port != 8080 {
		t.Errorf("expected port 9000 or default 8080, got %d", section.Port)
	}
	if section.Timeout != 20*time.Second && section.Timeout != 5*time.Second {
		t.Errorf("expected timeout 20s or default 5s, got %v", section.Timeout)
	}
}

func TestManager_Watch(t *testing.T) {
	tmpDir := t.TempDir()

	m := New(WithPath(tmpDir), WithEnvPrefix("TEST"))
	section := &testSection{}
	m.MustRegister(section)

	if err := m.Load(); err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	if err := m.Watch(100 * time.Millisecond); err != nil {
		t.Fatalf("Watch() failed: %v", err)
	}
	defer m.StopWatch()

	time.Sleep(50 * time.Millisecond)

	configFile := filepath.Join(tmpDir, "config.yml")
	yamlContent := `test:
  port: 9999
`
	if err := os.WriteFile(configFile, []byte(yamlContent), 0o644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	m.mu.RLock()
	port := section.Port
	m.mu.RUnlock()
	if port != 9999 {
		t.Errorf("expected port 9999 after reload, got %d", port)
	}
}

func TestManager_StopWatch(t *testing.T) {
	tmpDir := t.TempDir()

	m := New(WithPath(tmpDir), WithEnvPrefix("TEST"))
	section := &testSection{}
	m.MustRegister(section)

	if err := m.Load(); err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	if err := m.Watch(100 * time.Millisecond); err != nil {
		t.Fatalf("Watch() failed: %v", err)
	}

	m.StopWatch()

	// Calling StopWatch twice must not panic.
	m.StopWatch()
}

func TestManager_Watch_RejectsDoubleCall(t *testing.T) {
	tmpDir := t.TempDir()

	m := New(WithPath(tmpDir), WithEnvPrefix("TEST"))
	section := &testSection{}
	m.MustRegister(section)

	if err := m.Load(); err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	if err := m.Watch(100 * time.Millisecond); err != nil {
		t.Fatalf("Watch() failed: %v", err)
	}

	err := m.Watch(100 * time.Millisecond)
	if err == nil {
		t.Fatal("expected error on second Watch() call, got nil")
	}
	if !strings.Contains(err.Error(), "already running") {
		t.Errorf("expected 'already running' error, got: %v", err)
	}

	m.StopWatch()

	// After StopWatch(), a new Watch() call must return an error (nil channel).
	err = m.Watch(100 * time.Millisecond)
	if err == nil {
		t.Fatal("expected error on Watch() after StopWatch(), got nil")
	}
}

func TestPopulateEnv_EmptyPrefix(t *testing.T) {
	tmpDir := t.TempDir()

	m := New(WithPath(tmpDir))
	section := &testSection{}
	m.MustRegister(section)

	// Without globalPrefix, ENV keys are formed as SECTIONKEY_FIELD.
	t.Setenv("TEST_PORT", "5555")
	t.Setenv("TEST_HOST", "no-prefix-host")

	if err := m.Load(); err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	if section.Port != 5555 {
		t.Errorf("expected port 5555, got %d", section.Port)
	}
	if section.Host != "no-prefix-host" {
		t.Errorf("expected host 'no-prefix-host', got %q", section.Host)
	}
}

func TestManager_LockedFields(t *testing.T) {
	tmpDir := t.TempDir()

	yamlContent := `test:
  host: "yaml-host"
  port: 9000
`
	configFile := filepath.Join(tmpDir, "config.yml")
	if err := os.WriteFile(configFile, []byte(yamlContent), 0o644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	t.Setenv("TEST_TEST_TIMEOUT", "30s")

	m := New(WithPath(tmpDir), WithEnvPrefix("TEST"))
	section := &testSection{}
	m.MustRegister(section)

	if err := m.Load(); err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	locks := m.LockedFields("test")
	if locks == nil {
		t.Fatal("expected non-nil locked fields")
	}

	// YAML fields must be locked with source "yaml".
	if locks["host"] != "yaml" {
		t.Errorf("expected host locked by 'yaml', got %q", locks["host"])
	}
	if locks["port"] != "yaml" {
		t.Errorf("expected port locked by 'yaml', got %q", locks["port"])
	}

	// ENV fields must be locked with source "env".
	if locks["timeout"] != "env" {
		t.Errorf("expected timeout locked by 'env', got %q", locks["timeout"])
	}
}

package tune

import (
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

// TestManager_UnchangedSectionNotReloaded verifies that when one section
// changes, other sections do not receive an OnUpdate() call.
// Prevents unnecessary restarts of expensive components.
func TestManager_UnchangedSectionNotReloaded(t *testing.T) {
	tmpDir := t.TempDir()

	initialConfig := `logger:
  level: info
  format: text
server:
  listen_addr: "0.0.0.0:8080"
  buffer_size: 32768
`
	configFile := filepath.Join(tmpDir, "config.yml")
	if err := os.WriteFile(configFile, []byte(initialConfig), 0o644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	m := New(WithPath(configFile), WithEnvPrefix("MYAPP"))

	loggerSection := &mockLoggerSection{}
	serverSection := &mockServerSection{}

	m.MustRegister(loggerSection)
	m.MustRegister(serverSection)

	if err := m.Load(); err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	if err := m.Watch(100 * time.Millisecond); err != nil {
		t.Fatalf("Watch() failed: %v", err)
	}
	defer m.StopWatch()

	loggerSection.updateCount.Store(0)
	serverSection.updateCount.Store(0)

	time.Sleep(100 * time.Millisecond)

	// Change ONLY the logger section (level: info -> debug)
	updatedConfig := `logger:
  level: debug
  format: text
server:
  listen_addr: "0.0.0.0:8080"
  buffer_size: 32768
`
	if err := os.WriteFile(configFile, []byte(updatedConfig), 0o644); err != nil {
		t.Fatalf("failed to update config: %v", err)
	}

	now := time.Now()
	if err := os.Chtimes(configFile, now, now); err != nil {
		t.Fatalf("failed to update file mtime: %v", err)
	}

	t.Logf("Logger config changed (info -> debug), waiting for watcher...")

	time.Sleep(800 * time.Millisecond)

	loggerCount := loggerSection.updateCount.Load()
	serverCount := serverSection.updateCount.Load()

	if loggerCount == 0 {
		t.Error("logger.OnUpdate() should have been called (logger config changed)")
	}

	// CRITICAL: Server must NOT receive OnUpdate when its config is unchanged.
	if serverCount != 0 {
		t.Errorf("server.OnUpdate() should NOT have been called (server config unchanged), but was called %d times", serverCount)
	}

	// Read under lock because the watcher writes via copyFieldValues under m.mu.Lock
	m.mu.RLock()
	level := loggerSection.Level
	listenAddr := serverSection.ListenAddr
	m.mu.RUnlock()
	if level != "debug" {
		t.Errorf("logger.Level: expected 'debug', got %q", level)
	}
	if listenAddr != "0.0.0.0:8080" {
		t.Errorf("server.ListenAddr: expected '0.0.0.0:8080', got %q", listenAddr)
	}

	t.Logf("logger.OnUpdate() called %d times (expected > 0)", loggerCount)
	t.Logf("server.OnUpdate() called %d times (expected = 0) - server not restarted!", serverCount)
}

// mockLoggerSection is a simplified logger config section for testing.
type mockLoggerSection struct {
	Level       string `yaml:"level"`
	Format      string `yaml:"format"`
	updateCount atomic.Int32
}

func (m *mockLoggerSection) ConfigKey() string { return "logger" }
func (m *mockLoggerSection) SetDefaults()      {}
func (m *mockLoggerSection) Validate() error   { return nil }
func (m *mockLoggerSection) OnUpdate() {
	m.updateCount.Add(1)
}

// mockServerSection is a simplified server config section for testing.
type mockServerSection struct {
	ListenAddr  string `yaml:"listen_addr"`
	BufferSize  int    `yaml:"buffer_size"`
	updateCount atomic.Int32
}

func (m *mockServerSection) ConfigKey() string { return "server" }
func (m *mockServerSection) SetDefaults()      {}
func (m *mockServerSection) Validate() error   { return nil }
func (m *mockServerSection) OnUpdate() {
	m.updateCount.Add(1)
}

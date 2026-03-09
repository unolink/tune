package tune

import (
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

// TestManager_SelectiveOnUpdate verifies that OnUpdate() is called
// only for sections whose configuration actually changed during hot-reload.
func TestManager_SelectiveOnUpdate(t *testing.T) {
	tmpDir := t.TempDir()

	initialConfig := `section1:
  value: "initial1"
section2:
  value: "initial2"
`
	configFile := filepath.Join(tmpDir, "config.yml")
	if err := os.WriteFile(configFile, []byte(initialConfig), 0o644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	m := New(WithPath(configFile), WithEnvPrefix("TEST"))

	section1 := &trackingSection1{}
	section2 := &trackingSection2{}

	m.MustRegister(section1)
	m.MustRegister(section2)

	if err := m.Load(); err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	if section1.Value != "initial1" {
		t.Errorf("section1.Value: expected 'initial1', got %q", section1.Value)
	}
	if section2.Value != "initial2" {
		t.Errorf("section2.Value: expected 'initial2', got %q", section2.Value)
	}

	if err := m.Watch(100 * time.Millisecond); err != nil {
		t.Fatalf("Watch() failed: %v", err)
	}
	defer m.StopWatch()

	section1.updateCount.Store(0)
	section2.updateCount.Store(0)

	// Brief pause to ensure the file ModTime will differ after rewrite
	time.Sleep(100 * time.Millisecond)

	// Change ONLY section1
	updatedConfig := `section1:
  value: "updated1"
section2:
  value: "initial2"
`
	if err := os.WriteFile(configFile, []byte(updatedConfig), 0o644); err != nil {
		t.Fatalf("failed to update config: %v", err)
	}

	now := time.Now()
	if err := os.Chtimes(configFile, now, now); err != nil {
		t.Fatalf("failed to update file mtime: %v", err)
	}

	t.Logf("Config file updated, waiting for watcher...")

	time.Sleep(800 * time.Millisecond)

	count1 := section1.updateCount.Load()
	count2 := section2.updateCount.Load()

	if count1 == 0 {
		t.Error("section1.OnUpdate() should have been called (section changed)")
	}
	if count2 != 0 {
		t.Errorf("section2.OnUpdate() should NOT have been called (section unchanged), but was called %d times", count2)
	}

	// Read under lock because the watcher writes via copyFieldValues under m.mu.Lock
	m.mu.RLock()
	val1 := section1.Value
	val2 := section2.Value
	m.mu.RUnlock()
	if val1 != "updated1" {
		t.Errorf("section1.Value: expected 'updated1', got %q", val1)
	}
	if val2 != "initial2" {
		t.Errorf("section2.Value: expected 'initial2', got %q", val2)
	}

	t.Logf("section1.OnUpdate() called %d times (expected > 0)", count1)
	t.Logf("section2.OnUpdate() called %d times (expected = 0)", count2)
}

// trackingSection1 is a test config section that tracks OnUpdate calls.
type trackingSection1 struct {
	Value       string `yaml:"value"`
	updateCount atomic.Int32
}

func (t *trackingSection1) ConfigKey() string {
	return "section1"
}

func (t *trackingSection1) SetDefaults()    {}
func (t *trackingSection1) Validate() error { return nil }
func (t *trackingSection1) OnUpdate() {
	t.updateCount.Add(1)
}

// trackingSection2 is a test config section that tracks OnUpdate calls.
type trackingSection2 struct {
	Value       string `yaml:"value"`
	updateCount atomic.Int32
}

func (t *trackingSection2) ConfigKey() string {
	return "section2"
}

func (t *trackingSection2) SetDefaults()    {}
func (t *trackingSection2) Validate() error { return nil }
func (t *trackingSection2) OnUpdate() {
	t.updateCount.Add(1)
}

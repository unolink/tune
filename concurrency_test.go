package tune

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestManager_ConcurrentLoadAndRead(t *testing.T) {
	tmpDir := t.TempDir()

	m := New(WithPath(tmpDir), WithEnvPrefix("TEST"))
	section := &testSection{}
	m.MustRegister(section)

	configFile := filepath.Join(tmpDir, "config.yml")
	initialConfig := `test:
  port: 1000
`
	if err := os.WriteFile(configFile, []byte(initialConfig), 0o644); err != nil {
		t.Fatalf("failed to write initial config: %v", err)
	}

	if err := m.Load(); err != nil {
		t.Fatalf("initial Load() failed: %v", err)
	}

	if err := m.Watch(50 * time.Millisecond); err != nil {
		t.Fatalf("Watch() failed: %v", err)
	}
	defer m.StopWatch()

	var wg sync.WaitGroup
	readErrors := make(chan error, 100)
	readCount := 0
	var readMu sync.Mutex

	// Reader goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			m.mu.RLock()
			port := section.Port
			m.mu.RUnlock()

			if port <= 0 {
				readErrors <- &ReadError{Port: port}
			}

			readMu.Lock()
			readCount++
			readMu.Unlock()

			time.Sleep(10 * time.Millisecond)
		}
	}()

	// Writer goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 1; i <= 5; i++ {
			time.Sleep(100 * time.Millisecond)

			updatedConfig := `test:
  port: ` + fmt.Sprintf("%d", 2000+i) + `
`
			if err := os.WriteFile(configFile, []byte(updatedConfig), 0o644); err != nil {
				readErrors <- err
				return
			}
		}
	}()

	wg.Wait()
	time.Sleep(200 * time.Millisecond)

	close(readErrors)

	for err := range readErrors {
		if err != nil {
			t.Errorf("read error: %v", err)
		}
	}

	readMu.Lock()
	if readCount < 50 {
		t.Errorf("expected at least 50 reads, got %d", readCount)
	}
	readMu.Unlock()
}

func TestManager_ConcurrentRegisterAndLoad(t *testing.T) {
	tmpDir := t.TempDir()

	m := New(WithPath(tmpDir), WithEnvPrefix("TEST"))

	var wg sync.WaitGroup
	errors := make(chan error, 100)

	// Registration goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 10; i++ {
			section := &concurrentTestSection{ID: i}
			m.MustRegister(section)
			time.Sleep(5 * time.Millisecond)
		}
	}()

	// Load goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 10; i++ {
			if err := m.Load(); err != nil {
				errors <- err
			}
			time.Sleep(5 * time.Millisecond)
		}
	}()

	wg.Wait()
	time.Sleep(50 * time.Millisecond)

	close(errors)

	for err := range errors {
		if err != nil {
			t.Errorf("concurrent operation error: %v", err)
		}
	}
}

// ReadError represents an invalid port value read during concurrent access.
type ReadError struct {
	Port int
}

func (e *ReadError) Error() string {
	return "invalid port read"
}

// concurrentTestSection is a test config section for concurrency tests.
type concurrentTestSection struct {
	Name string `yaml:"name"`
	ID   int    `yaml:"id"`
}

func (c *concurrentTestSection) ConfigKey() string {
	return "concurrent"
}
func (c *concurrentTestSection) SetDefaults() {
	c.Name = "default"
}
func (c *concurrentTestSection) Validate() error { return nil }
func (c *concurrentTestSection) OnUpdate()       {}

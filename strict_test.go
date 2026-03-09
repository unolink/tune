package tune

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func TestDecodeSection_StrictMode_ValidFields(t *testing.T) {
	t.Parallel()
	// YAML data with only valid fields.
	sectionData := map[string]any{
		"port":    8080,
		"host":    "localhost",
		"timeout": "5s",
		"enabled": true,
	}

	rootNode := &yaml.Node{}
	if err := rootNode.Encode(map[string]any{"test": sectionData}); err != nil {
		t.Fatalf("failed to encode test data: %v", err)
	}

	section := &testSection{}
	err := decodeSection(rootNode, "test", section)

	if err != nil {
		t.Fatalf("unexpected error for valid fields: %v", err)
	}

	// Verify that values were loaded correctly.
	if section.Port != 8080 {
		t.Errorf("expected port 8080, got %d", section.Port)
	}
	if section.Host != "localhost" {
		t.Errorf("expected host localhost, got %q", section.Host)
	}
}

func TestDiff_Basic(t *testing.T) {
	t.Parallel()
	oldSection := &testSection{}
	oldSection.Port = 8080
	oldSection.Host = "localhost"
	oldSection.Timeout = 5 * time.Second
	oldSection.Enabled = true

	newSection := &testSection{}
	newSection.Port = 9000
	newSection.Host = "0.0.0.0"
	newSection.Timeout = 10 * time.Second
	newSection.Enabled = false

	changes := Diff(oldSection, newSection)

	if len(changes) != 4 {
		t.Errorf("expected 4 changes, got %d: %v", len(changes), changes)
	}

	// Verify that all expected field changes are present.
	changesStr := strings.Join(changes, " ")
	if !strings.Contains(changesStr, "port") {
		t.Error("expected port change")
	}
	if !strings.Contains(changesStr, "host") {
		t.Error("expected host change")
	}
	if !strings.Contains(changesStr, "timeout") {
		t.Error("expected timeout change")
	}
	if !strings.Contains(changesStr, "enabled") {
		t.Error("expected enabled change")
	}
}

func TestDiff_NoChanges(t *testing.T) {
	t.Parallel()
	section1 := &testSection{}
	section1.Port = 8080
	section1.Host = "localhost"

	section2 := &testSection{}
	section2.Port = 8080
	section2.Host = "localhost"

	changes := Diff(section1, section2)

	if len(changes) != 0 {
		t.Errorf("expected no changes, got %d: %v", len(changes), changes)
	}
}

func TestDiff_SecretFields(t *testing.T) {
	t.Parallel()
	oldSection := &testUsageSection{}
	oldSection.Password = "old-secret"

	newSection := &testUsageSection{}
	newSection.Password = "new-secret"

	changes := Diff(oldSection, newSection)

	// Change must be reported but without exposing actual values.
	if len(changes) == 0 {
		t.Error("expected change for secret field")
	}

	// Secret values must not appear in the diff output.
	changesStr := strings.Join(changes, " ")
	if strings.Contains(changesStr, "old-secret") || strings.Contains(changesStr, "new-secret") {
		t.Error("secret values should not be shown in diff")
	}

	// The fact of the change must still be indicated.
	if !strings.Contains(changesStr, "password") || !strings.Contains(changesStr, "changed") {
		t.Error("expected password change indication without values")
	}
}

func TestDiff_NestedStructures(t *testing.T) {
	t.Parallel()
	oldSection := &testNestedSection{}
	oldSection.Server.Host = "localhost"
	oldSection.Server.Port = 8080
	oldSection.Database.MaxConn = 10

	newSection := &testNestedSection{}
	newSection.Server.Host = "0.0.0.0"
	newSection.Server.Port = 9000
	newSection.Database.MaxConn = 20

	changes := Diff(oldSection, newSection)

	if len(changes) < 3 {
		t.Errorf("expected at least 3 changes, got %d: %v", len(changes), changes)
	}

	changesStr := strings.Join(changes, " ")
	if !strings.Contains(changesStr, "server.host") {
		t.Error("expected server.host change")
	}
	if !strings.Contains(changesStr, "server.port") {
		t.Error("expected server.port change")
	}
	if !strings.Contains(changesStr, "database.max_conn") {
		t.Error("expected database.max_conn change")
	}
}

func TestWatch_LogsChanges(t *testing.T) {
	tmpDir := t.TempDir()

	m := New(WithPath(tmpDir), WithEnvPrefix("TEST"))
	section := &testSection{}
	m.MustRegister(section)

	if err := m.Load(); err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	// Custom logger to capture reload messages.
	var loggedMessages []string
	testLogger := &testLogger{messages: &loggedMessages}

	m.SetLogger(testLogger)

	if err := m.Watch(100 * time.Millisecond); err != nil {
		t.Fatalf("Watch() failed: %v", err)
	}
	defer m.StopWatch()

	time.Sleep(50 * time.Millisecond)

	configFile := filepath.Join(tmpDir, "config.yml")
	yamlContent := `test:
  port: 9999
  host: "0.0.0.0"
`
	if err := os.WriteFile(configFile, []byte(yamlContent), 0o644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	messages := testLogger.getMessages()
	if len(messages) == 0 {
		t.Error("expected logged messages about config changes")
	}

	// Log output must contain field-level change details.
	loggedStr := strings.Join(messages, " ")
	if !strings.Contains(loggedStr, "changed") && !strings.Contains(loggedStr, "port") {
		t.Error("expected change information in logs")
	}
}

// testLogger captures log messages for assertion in tests.
type testLogger struct {
	messages *[]string
	mu       sync.Mutex
}

func (l *testLogger) InfoContext(_ context.Context, msg string, args ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()

	*l.messages = append(*l.messages, msg)

	for i := 0; i < len(args); i += 2 {
		if i+1 < len(args) {
			*l.messages = append(*l.messages, fmt.Sprintf("%v=%v", args[i], args[i+1]))
		} else {
			*l.messages = append(*l.messages, fmt.Sprintf("%v", args[i]))
		}
	}
}

func (l *testLogger) getMessages() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	result := make([]string, len(*l.messages))
	copy(result, *l.messages)
	return result
}

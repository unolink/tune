package tune

import (
	"os"
	"path/filepath"
	"testing"
)

// TestManager_LoadWithMultipleBackends verifies loading a configuration
// with multiple backends in an array.
func TestManager_LoadWithMultipleBackends(t *testing.T) {
	tmpDir := t.TempDir()

	yamlContent := `gateway:
  listen_addr: "0.0.0.0:8080"
  read_timeout: 5s
  write_timeout: 30s
  max_connections: 32768
  backends:
    - name: "backend-primary"
      address: "192.168.1.33:8080"
      weight: 100
      routes:
        - path: "/api/v1"
    - name: "backend-secondary"
      address: "192.168.1.32:8080"
      weight: 50
      routes:
        - path: "/api/v2"
`
	configFile := filepath.Join(tmpDir, "config.yml")
	if err := os.WriteFile(configFile, []byte(yamlContent), 0o644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	m := New(WithPath(tmpDir), WithEnvPrefix("MYAPP"))
	gatewaySection := &testGatewaySection{}
	m.MustRegister(gatewaySection)

	if err := m.Load(); err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	if gatewaySection.ListenAddr != "0.0.0.0:8080" {
		t.Errorf("expected listen_addr '0.0.0.0:8080', got %q", gatewaySection.ListenAddr)
	}

	// CRITICAL: must load 2 backends, not 1
	if len(gatewaySection.Backends) != 2 {
		t.Errorf("expected 2 backends, got %d", len(gatewaySection.Backends))
		t.Logf("Loaded backends: %+v", gatewaySection.Backends)
	}

	if len(gatewaySection.Backends) > 0 {
		b1 := gatewaySection.Backends[0]
		if b1.Name != "backend-primary" {
			t.Errorf("backend[0].Name: expected 'backend-primary', got %q", b1.Name)
		}
		if b1.Address != "192.168.1.33:8080" {
			t.Errorf("backend[0].Address: expected '192.168.1.33:8080', got %q", b1.Address)
		}
		if b1.Weight != 100 {
			t.Errorf("backend[0].Weight: expected 100, got %d", b1.Weight)
		}
		if len(b1.Routes) != 1 || b1.Routes[0].Path != "/api/v1" {
			t.Errorf("backend[0].Routes: expected ['/api/v1'], got %v", b1.Routes)
		}
	}

	if len(gatewaySection.Backends) > 1 {
		b2 := gatewaySection.Backends[1]
		if b2.Name != "backend-secondary" {
			t.Errorf("backend[1].Name: expected 'backend-secondary', got %q", b2.Name)
		}
		if b2.Address != "192.168.1.32:8080" {
			t.Errorf("backend[1].Address: expected '192.168.1.32:8080', got %q", b2.Address)
		}
		if b2.Weight != 50 {
			t.Errorf("backend[1].Weight: expected 50, got %d", b2.Weight)
		}
		if len(b2.Routes) != 1 || b2.Routes[0].Path != "/api/v2" {
			t.Errorf("backend[1].Routes: expected ['/api/v2'], got %v", b2.Routes)
		}
	}
}

// TestManager_LoadWithSingleBackend verifies the basic single-backend case.
func TestManager_LoadWithSingleBackend(t *testing.T) {
	tmpDir := t.TempDir()

	yamlContent := `gateway:
  listen_addr: "0.0.0.0:8080"
  backends:
    - name: "test-backend"
      address: "localhost:9090"
      weight: 100
      routes:
        - path: "/healthz"
          auth: "bearer"
`
	configFile := filepath.Join(tmpDir, "config.yml")
	if err := os.WriteFile(configFile, []byte(yamlContent), 0o644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	m := New(WithPath(tmpDir), WithEnvPrefix("MYAPP"))
	gatewaySection := &testGatewaySection{}
	m.MustRegister(gatewaySection)

	if err := m.Load(); err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	if len(gatewaySection.Backends) != 1 {
		t.Errorf("expected 1 backend, got %d", len(gatewaySection.Backends))
	}

	if len(gatewaySection.Backends) > 0 {
		backend := gatewaySection.Backends[0]
		if backend.Name != "test-backend" {
			t.Errorf("expected name 'test-backend', got %q", backend.Name)
		}
		if backend.Address != "localhost:9090" {
			t.Errorf("expected address 'localhost:9090', got %q", backend.Address)
		}
	}
}

// testGatewaySection is a configuration section with nested arrays for testing.
type testGatewaySection struct {
	ListenAddr     string              `yaml:"listen_addr"`
	ReadTimeoutStr string              `yaml:"read_timeout"`
	WriteTimeout   string              `yaml:"write_timeout"`
	Backends       []testBackendConfig `yaml:"backends"`
	MaxConnections int                 `yaml:"max_connections"`
}

type testBackendConfig struct {
	Name    string            `yaml:"name"`
	Address string            `yaml:"address"`
	Weight  int               `yaml:"weight"`
	Routes  []testRouteConfig `yaml:"routes"`
}

type testRouteConfig struct {
	Path   string `yaml:"path"`
	Auth   string `yaml:"auth"`
	Secret string `yaml:"secret"`
}

func (g *testGatewaySection) ConfigKey() string {
	return "gateway"
}

func (g *testGatewaySection) SetDefaults() {
	if g.ReadTimeoutStr == "" {
		g.ReadTimeoutStr = "5s"
	}
	if g.WriteTimeout == "" {
		g.WriteTimeout = "30s"
	}
	if g.MaxConnections == 0 {
		g.MaxConnections = 32768
	}
}

func (g *testGatewaySection) Validate() error {
	if g.ListenAddr == "" {
		return &ValidationError{Section: "gateway", Message: "listen_addr is required"}
	}
	if len(g.Backends) == 0 {
		return &ValidationError{Section: "gateway", Message: "at least one backend is required"}
	}
	return nil
}

func (g *testGatewaySection) OnUpdate() {
	// No-op for test
}

// TestManager_LoadWithDuplicateNames verifies that duplicate backend names
// do not cause data loss during YAML array deserialization.
func TestManager_LoadWithDuplicateNames(t *testing.T) {
	tmpDir := t.TempDir()

	yamlContent := `gateway:
  listen_addr: "0.0.0.0:8080"
  backends:
    - name: "backend-alpha"
      address: "192.168.1.33:8080"
      weight: 100
      routes:
        - path: "/api/v1"
    - name: "backend-alpha"
      address: "192.168.1.32:9090"
      weight: 50
      routes:
        - path: "/api/v2"
`
	configFile := filepath.Join(tmpDir, "config.yml")
	if err := os.WriteFile(configFile, []byte(yamlContent), 0o644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	m := New(WithPath(tmpDir), WithEnvPrefix("MYAPP"))
	gatewaySection := &testGatewaySection{}
	m.MustRegister(gatewaySection)

	if err := m.Load(); err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	// Both entries must be preserved even with duplicate names (YAML arrays keep all elements).
	if len(gatewaySection.Backends) != 2 {
		t.Errorf("Expected 2 backends with duplicate names, got %d", len(gatewaySection.Backends))
	}

	if len(gatewaySection.Backends) == 2 {
		t.Logf("Config loaded 2 backends correctly:")
		t.Logf("  Backend[0]: %s @ %s", gatewaySection.Backends[0].Name, gatewaySection.Backends[0].Address)
		t.Logf("  Backend[1]: %s @ %s", gatewaySection.Backends[1].Name, gatewaySection.Backends[1].Address)
	}
}

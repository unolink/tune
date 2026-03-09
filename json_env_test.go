package tune

import (
	"os"
	"strings"
	"testing"
)

// testJSONSection is a test config section for JSON ENV functionality.
type testJSONSection struct {
	Endpoints []EndpointConfig `yaml:"endpoints"`
	Options   struct {
		Message string `yaml:"message"`
		Timeout int    `yaml:"timeout"`
		Debug   bool   `yaml:"debug"`
	} `yaml:"options"`
}

type EndpointConfig struct {
	Name    string        `yaml:"name" json:"name"`
	Address string        `yaml:"address" json:"address"`
	Routes  []RouteConfig `yaml:"routes" json:"routes"`
}

type RouteConfig struct {
	Name     string `yaml:"name" json:"name"`
	AuthUser string `yaml:"auth_user" json:"auth_user"`
	AuthKey  string `yaml:"auth_key" json:"auth_key"`
}

func (t *testJSONSection) ConfigKey() string { return "test_json" }
func (t *testJSONSection) SetDefaults() {
	t.Options.Timeout = 30
	t.Options.Debug = false
}
func (t *testJSONSection) Validate() error { return nil }
func (t *testJSONSection) OnUpdate()       {}

func TestPopulateEnv_JSONSlice(t *testing.T) {
	tmpDir := t.TempDir()
	m := New(WithPath(tmpDir), WithEnvPrefix("MYAPP"))

	section := &testJSONSection{}
	m.MustRegister(section)

	// JSON ENV variable with a list of endpoints.
	jsonValue := `[
  {
    "name": "prod",
    "address": "192.168.1.10:8080",
    "routes": [
      {"name": "users-api", "auth_user": "admin", "auth_key": "${PROD_AUTH_KEY}"},
      {"name": "orders-api"}
    ]
  },
  {
    "name": "dev",
    "address": "127.0.0.1:9090",
    "routes": [
      {"name": "dev-api", "auth_key": "${DEV_AUTH_KEY}"}
    ]
  }
]`
	t.Setenv("MYAPP_TEST_JSON_ENDPOINTS", jsonValue)

	// Secrets for variable expansion.
	t.Setenv("PROD_AUTH_KEY", "SuperSecretProdKey")
	t.Setenv("DEV_AUTH_KEY", "DevKey123")

	if err := m.Load(); err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	if len(section.Endpoints) != 2 {
		t.Fatalf("expected 2 endpoints, got %d", len(section.Endpoints))
	}

	if section.Endpoints[0].Name != "prod" {
		t.Errorf("expected endpoint name 'prod', got %q", section.Endpoints[0].Name)
	}
	if section.Endpoints[0].Address != "192.168.1.10:8080" {
		t.Errorf("expected address '192.168.1.10:8080', got %q", section.Endpoints[0].Address)
	}

	if len(section.Endpoints[0].Routes) != 2 {
		t.Errorf("expected 2 routes, got %d", len(section.Endpoints[0].Routes))
	}
	if section.Endpoints[0].Routes[0].Name != "users-api" {
		t.Errorf("expected route 'users-api', got %q", section.Endpoints[0].Routes[0].Name)
	}
	if section.Endpoints[0].Routes[0].AuthUser != "admin" {
		t.Errorf("expected auth_user 'admin', got %q", section.Endpoints[0].Routes[0].AuthUser)
	}

	// Variable expansion must resolve ${...} references.
	if section.Endpoints[0].Routes[0].AuthKey != "SuperSecretProdKey" {
		t.Errorf("expected auth_key 'SuperSecretProdKey', got %q", section.Endpoints[0].Routes[0].AuthKey)
	}

	if section.Endpoints[1].Name != "dev" {
		t.Errorf("expected endpoint name 'dev', got %q", section.Endpoints[1].Name)
	}
	if section.Endpoints[1].Routes[0].AuthKey != "DevKey123" {
		t.Errorf("expected auth_key 'DevKey123', got %q", section.Endpoints[1].Routes[0].AuthKey)
	}
}

func TestPopulateEnv_JSONStruct(t *testing.T) {
	tmpDir := t.TempDir()
	m := New(WithPath(tmpDir), WithEnvPrefix("MYAPP"))

	section := &testJSONSection{}
	m.MustRegister(section)

	// JSON ENV variable for a nested struct.
	jsonValue := `{
  "timeout": 60,
  "debug": true,
  "message": "Hello ${USER_NAME}"
}`
	t.Setenv("MYAPP_TEST_JSON_OPTIONS", jsonValue)
	t.Setenv("USER_NAME", "TestUser")

	if err := m.Load(); err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	if section.Options.Timeout != 60 {
		t.Errorf("expected timeout 60, got %d", section.Options.Timeout)
	}
	if section.Options.Debug != true {
		t.Errorf("expected debug true, got %v", section.Options.Debug)
	}

	// Variable expansion in JSON struct values.
	if section.Options.Message != "Hello TestUser" {
		t.Errorf("expected message 'Hello TestUser', got %q", section.Options.Message)
	}
}

func TestPopulateEnv_InvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	m := New(WithPath(tmpDir), WithEnvPrefix("MYAPP"))

	section := &testJSONSection{}
	m.MustRegister(section)

	// Invalid JSON value.
	t.Setenv("MYAPP_TEST_JSON_ENDPOINTS", `[invalid json`)

	err := m.Load()
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}

	if !strings.Contains(err.Error(), "failed to parse JSON") {
		t.Errorf("expected JSON parse error, got: %v", err)
	}
}

func TestExpandEnv_SimpleString(t *testing.T) {
	tmpDir := t.TempDir()
	m := New(WithPath(tmpDir), WithEnvPrefix("MYAPP"))

	section := &testSection{}
	m.MustRegister(section)

	// ENV variable with embedded variable expansion.
	t.Setenv("MYAPP_TEST_HOST", "localhost:${PORT}")
	t.Setenv("PORT", "8080")

	if err := m.Load(); err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	if section.Host != "localhost:8080" {
		t.Errorf("expected host 'localhost:8080', got %q", section.Host)
	}
}

func TestExpandEnv_YAMLWithVariables(t *testing.T) {
	tmpDir := t.TempDir()

	yamlContent := `test:
  host: "localhost:${PORT}"
  timeout: "5s"
`
	configFile := tmpDir + "/config.yml"
	if err := os.WriteFile(configFile, []byte(yamlContent), 0o644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	t.Setenv("PORT", "9999")

	m := New(WithPath(tmpDir), WithEnvPrefix("MYAPP"))
	section := &testSection{}
	m.MustRegister(section)

	if err := m.Load(); err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	// Variable expansion must work in YAML string values too.
	if section.Host != "localhost:9999" {
		t.Errorf("expected host 'localhost:9999', got %q", section.Host)
	}
}

func TestExpandEnv_NestedInJSON(t *testing.T) {
	tmpDir := t.TempDir()
	m := New(WithPath(tmpDir), WithEnvPrefix("MYAPP"))

	section := &testJSONSection{}
	m.MustRegister(section)

	// JSON with variables in multiple nested positions.
	jsonValue := `[
  {
    "name": "prod",
    "address": "${PROD_HOST}:${PROD_PORT}",
    "routes": [
      {"name": "${ROUTE_1}", "auth_key": "${PROD_AUTH_KEY}"},
      {"name": "${ROUTE_2}"}
    ]
  }
]`
	t.Setenv("MYAPP_TEST_JSON_ENDPOINTS", jsonValue)
	t.Setenv("PROD_HOST", "192.168.1.10")
	t.Setenv("PROD_PORT", "8080")
	t.Setenv("PROD_AUTH_KEY", "Secret123")
	t.Setenv("ROUTE_1", "users-api")
	t.Setenv("ROUTE_2", "orders-api")

	if err := m.Load(); err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	// Variable expansion must work across all nested fields.
	if section.Endpoints[0].Address != "192.168.1.10:8080" {
		t.Errorf("expected address '192.168.1.10:8080', got %q", section.Endpoints[0].Address)
	}
	if len(section.Endpoints[0].Routes) != 2 {
		t.Fatalf("expected 2 routes, got %d", len(section.Endpoints[0].Routes))
	}
	if section.Endpoints[0].Routes[0].Name != "users-api" {
		t.Errorf("expected route 'users-api', got %q", section.Endpoints[0].Routes[0].Name)
	}
	if section.Endpoints[0].Routes[0].AuthKey != "Secret123" {
		t.Errorf("expected auth_key 'Secret123', got %q", section.Endpoints[0].Routes[0].AuthKey)
	}
	if section.Endpoints[0].Routes[1].Name != "orders-api" {
		t.Errorf("expected route 'orders-api', got %q", section.Endpoints[0].Routes[1].Name)
	}
}

func TestExpandEnv_DollarVarFormat(t *testing.T) {
	tmpDir := t.TempDir()
	m := New(WithPath(tmpDir), WithEnvPrefix("MYAPP"))

	section := &testSection{}
	m.MustRegister(section)

	// $VAR format (without braces) must also be supported.
	t.Setenv("MYAPP_TEST_HOST", "localhost:$PORT")
	t.Setenv("PORT", "8080")

	if err := m.Load(); err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	if section.Host != "localhost:8080" {
		t.Errorf("expected host 'localhost:8080', got %q", section.Host)
	}
}

// testMapSection is a test section with map[string]string to verify expansion in map values.
type testMapSection struct {
	Labels map[string]string `yaml:"labels"`
}

func (t *testMapSection) ConfigKey() string { return "test_map" }
func (t *testMapSection) SetDefaults()      {}
func (t *testMapSection) Validate() error   { return nil }
func (t *testMapSection) OnUpdate()         {}

func TestExpandEnv_MapValues(t *testing.T) {
	tmpDir := t.TempDir()

	yamlContent := `test_map:
  labels:
    env: "${MAP_ENV}"
    host: "${MAP_HOST}"
    static: "no-expand"
`
	configFile := tmpDir + "/config.yml"
	if err := os.WriteFile(configFile, []byte(yamlContent), 0o644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	t.Setenv("MAP_ENV", "production")
	t.Setenv("MAP_HOST", "db.example.com")

	m := New(WithPath(tmpDir), WithEnvPrefix("MYAPP"))
	section := &testMapSection{}
	m.MustRegister(section)

	if err := m.Load(); err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	if section.Labels["env"] != "production" {
		t.Errorf("expected labels[env]='production', got %q", section.Labels["env"])
	}
	if section.Labels["host"] != "db.example.com" {
		t.Errorf("expected labels[host]='db.example.com', got %q", section.Labels["host"])
	}
	if section.Labels["static"] != "no-expand" {
		t.Errorf("expected labels[static]='no-expand', got %q", section.Labels["static"])
	}
}

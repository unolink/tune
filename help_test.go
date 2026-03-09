package tune

import (
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

// testUsageSection is a test section for GetUsage verification.
type testUsageSection struct {
	Host        string        `yaml:"host" usage:"Host address to bind to"`
	Password    string        `yaml:"password" secret:"true" usage:"Admin password"`
	DatabaseURL string        `yaml:"database_url" usage:"Database connection URL"`
	Ignored     string        `yaml:"-"`
	unexported  string        //nolint:unused // accessed via reflect; tests that documentation skips unexported fields
	Port        int           `yaml:"port" usage:"Port to listen on for HTTP requests"`
	Timeout     time.Duration `yaml:"timeout" usage:"Request timeout"`
	Enabled     bool          `yaml:"enabled" usage:"Enable the server"`
}

func (t *testUsageSection) ConfigKey() string { return "server" }
func (t *testUsageSection) SetDefaults() {
	t.Port = 8080
	t.Host = "localhost"
	t.Timeout = 5 * time.Second
	t.Enabled = true
	t.Password = "default-secret"
	t.DatabaseURL = "postgres://localhost/db"
}
func (t *testUsageSection) Validate() error { return nil }
func (t *testUsageSection) OnUpdate()       {}

// testNestedSection is a test section with nested structs.
type testNestedSection struct {
	Server struct {
		Host string `yaml:"host" usage:"Server host"`
		Port int    `yaml:"port" usage:"Server port"`
	} `yaml:"server"`
	Database struct {
		DSN     string `yaml:"dsn" usage:"Database DSN" secret:"true"`
		MaxConn int    `yaml:"max_conn" usage:"Maximum connections"`
	} `yaml:"database"`
}

func (t *testNestedSection) ConfigKey() string { return "nested" }
func (t *testNestedSection) SetDefaults() {
	t.Server.Host = "0.0.0.0"
	t.Server.Port = 9000
	t.Database.DSN = "postgres://user:pass@localhost/db"
	t.Database.MaxConn = 10
}
func (t *testNestedSection) Validate() error { return nil }
func (t *testNestedSection) OnUpdate()       {}

// testArraySection is a test section with an array of structs.
type testArraySection struct {
	Servers []testServerConfig `yaml:"servers" usage:"List of servers to connect"`
}

type testServerConfig struct {
	Name    string   `yaml:"name" usage:"Server name"`
	Address string   `yaml:"address" usage:"Server address"`
	Aliases []string `yaml:"aliases" usage:"Server aliases"`
}

func (t *testArraySection) ConfigKey() string { return "array" }
func (t *testArraySection) SetDefaults()      {}
func (t *testArraySection) Validate() error   { return nil }
func (t *testArraySection) OnUpdate()         {}

func TestGetUsage_Basic(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	m := New(WithPath(tmpDir), WithEnvPrefix("MYAPP"))

	section := &testUsageSection{}
	m.MustRegister(section)

	usage := m.GetUsage()

	// Verify key elements are present.
	if !strings.Contains(usage, "SECTION: server") {
		t.Error("usage should contain section name")
	}

	if !strings.Contains(usage, "MYAPP_SERVER_") {
		t.Error("usage should contain ENV prefix")
	}

	if !strings.Contains(usage, "Port") {
		t.Error("usage should contain Port field")
	}

	if !strings.Contains(usage, "8080") {
		t.Error("usage should contain default port value")
	}

	if !strings.Contains(usage, "Port to listen on for HTTP requests") {
		t.Error("usage should contain usage description")
	}

	// Secret fields must be marked.
	if !strings.Contains(usage, "Secret:") {
		t.Error("usage should mark secret fields")
	}

	// Ignored fields must be absent.
	if strings.Contains(usage, "Ignored") {
		t.Error("usage should not contain ignored fields")
	}
}

func TestGetUsage_MultipleSections(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	m := New(WithPath(tmpDir), WithEnvPrefix("MYAPP"))

	section1 := &testUsageSection{}
	section2 := &testNestedSection{}

	m.MustRegister(section1)
	m.MustRegister(section2)

	usage := m.GetUsage()

	if !strings.Contains(usage, "SECTION: server") {
		t.Error("usage should contain server section")
	}

	if !strings.Contains(usage, "SECTION: nested") {
		t.Error("usage should contain nested section")
	}
}

func TestGetUsage_Empty(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	m := New(WithPath(tmpDir), WithEnvPrefix("MYAPP"))

	usage := m.GetUsage()

	if !strings.Contains(usage, "No configuration sections registered") {
		t.Error("usage should indicate no sections")
	}
}

func TestGetDocumentation_Basic(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	m := New(WithPath(tmpDir), WithEnvPrefix("MYAPP"))

	section := &testUsageSection{}
	m.MustRegister(section)

	docs := m.GetDocumentation()

	if len(docs) != 1 {
		t.Fatalf("expected 1 section, got %d", len(docs))
	}

	sectionDoc := docs[0]
	if sectionDoc.Key != "server" {
		t.Errorf("expected section key 'server', got %q", sectionDoc.Key)
	}

	if !strings.Contains(sectionDoc.ENVPrefix, "MYAPP_SERVER_") {
		t.Errorf("expected ENV prefix to contain MYAPP_SERVER_, got %q", sectionDoc.ENVPrefix)
	}

	if len(sectionDoc.Fields) == 0 {
		t.Fatal("expected at least one field")
	}

	foundPort := false
	foundHost := false
	for _, field := range sectionDoc.Fields {
		if field.Field == "Port" {
			foundPort = true
			if field.Type != "int" {
				t.Errorf("expected Port type 'int', got %q", field.Type)
			}
			if field.YAML != "port" {
				t.Errorf("expected Port YAML key 'port', got %q", field.YAML)
			}
			if !strings.Contains(field.ENV, "MYAPP_SERVER_PORT") {
				t.Errorf("expected Port ENV to contain MYAPP_SERVER_PORT, got %q", field.ENV)
			}
		}
		if field.Field == "Host" {
			foundHost = true
		}
		if field.Field == "Password" {
			if !field.IsSecret {
				t.Error("expected Password to be marked as secret")
			}
		}
	}

	if !foundPort {
		t.Error("expected to find Port field")
	}
	if !foundHost {
		t.Error("expected to find Host field")
	}
}

func TestGetDocumentation_MultipleSections(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	m := New(WithPath(tmpDir), WithEnvPrefix("MYAPP"))

	section1 := &testUsageSection{}
	section2 := &testNestedSection{}

	m.MustRegister(section1)
	m.MustRegister(section2)

	docs := m.GetDocumentation()

	if len(docs) != 2 {
		t.Fatalf("expected 2 sections, got %d", len(docs))
	}

	// Sections must be sorted by key.
	if docs[0].Key > docs[1].Key {
		t.Error("sections should be sorted by key")
	}
}

func TestGetDocumentation_Empty(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	m := New(WithPath(tmpDir), WithEnvPrefix("TEST"))

	docs := m.GetDocumentation()

	if len(docs) != 0 {
		t.Errorf("expected nil or empty slice, got %d sections", len(docs))
	}
}

func TestGetDocumentation_FieldDetails(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	m := New(WithPath(tmpDir), WithEnvPrefix("MYAPP"))

	section := &testUsageSection{}
	m.MustRegister(section)

	docs := m.GetDocumentation()
	if len(docs) == 0 {
		t.Fatal("expected non-empty documentation")
	}
	sectionDoc := docs[0]

	// Find the field with a usage description.
	var portField *DocField
	for i := range sectionDoc.Fields {
		if sectionDoc.Fields[i].Field == "Port" {
			portField = &sectionDoc.Fields[i]
			break
		}
	}

	if portField == nil {
		t.Fatal("expected to find Port field")
	}

	// Verify field details.
	if portField.Default == "" {
		t.Error("expected Port to have default value")
	}

	if portField.Usage == "" {
		t.Error("expected Port to have usage description")
	}

	if portField.ENV == "" {
		t.Error("expected Port to have ENV variable name")
	}

	if portField.YAML == "" {
		t.Error("expected Port to have YAML key")
	}
}

func TestGetDocumentation_SecretFields(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	m := New(WithPath(tmpDir), WithEnvPrefix("TEST"))

	section := &testUsageSection{}
	m.MustRegister(section)

	docs := m.GetDocumentation()
	if len(docs) == 0 {
		t.Fatal("expected non-empty documentation")
	}
	sectionDoc := docs[0]

	// Find the secret field.
	var passwordField *DocField
	for i := range sectionDoc.Fields {
		if sectionDoc.Fields[i].Field == "Password" {
			passwordField = &sectionDoc.Fields[i]
			break
		}
	}

	if passwordField == nil {
		t.Fatal("expected to find Password field")
	}

	if !passwordField.IsSecret {
		t.Error("expected Password field to be marked as secret")
	}
}

func TestGetDebugConfigYAML_Basic(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	m := New(WithPath(tmpDir), WithEnvPrefix("MYAPP"))

	section := &testUsageSection{}
	m.MustRegister(section)

	if err := m.Load(); err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	section.Port = 9999
	section.Host = "0.0.0.0"
	section.Password = "SuperSecretPassword123"

	debug, err := m.GetDebugConfigYAML()
	if err != nil {
		t.Fatalf("GetDebugConfigYAML() failed: %v", err)
	}

	if !strings.Contains(debug, "server:") {
		t.Error("debug config should contain section")
	}

	if !strings.Contains(debug, "port:") {
		t.Error("debug config should contain port field")
	}

	if !strings.Contains(debug, "9999") {
		t.Error("debug config should contain port value")
	}

	// Secrets must be redacted.
	if strings.Contains(debug, "SuperSecretPassword123") {
		t.Error("debug config should not contain actual secret value")
	}

	if !strings.Contains(debug, "<REDACTED>") {
		t.Error("debug config should contain redacted placeholder")
	}
}

func TestGetDebugConfigYAML_Nested(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	m := New(WithPath(tmpDir), WithEnvPrefix("MYAPP"))

	section := &testNestedSection{}
	m.MustRegister(section)

	if err := m.Load(); err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	section.Database.DSN = "postgres://user:secret@localhost/db"

	debug, err := m.GetDebugConfigYAML()
	if err != nil {
		t.Fatalf("GetDebugConfigYAML() failed: %v", err)
	}

	if !strings.Contains(debug, "database:") {
		t.Error("debug config should contain database section")
	}

	// Secrets must be redacted.
	if strings.Contains(debug, "secret") {
		t.Error("debug config should not contain actual secret value")
	}

	if !strings.Contains(debug, "<REDACTED>") {
		t.Error("debug config should contain redacted placeholder")
	}
}

func TestGetDebugConfigYAML_WithENV(t *testing.T) {
	tmpDir := t.TempDir()
	m := New(WithPath(tmpDir), WithEnvPrefix("MYAPP"))

	section := &testUsageSection{}
	m.MustRegister(section)

	t.Setenv("MYAPP_SERVER_PORT", "7777")
	t.Setenv("MYAPP_SERVER_PASSWORD", "EnvSecret123")

	if err := m.Load(); err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	debug, err := m.GetDebugConfigYAML()
	if err != nil {
		t.Fatalf("GetDebugConfigYAML() failed: %v", err)
	}

	// ENV values must be present.
	if !strings.Contains(debug, "7777") {
		t.Error("debug config should contain ENV port value")
	}

	// Secrets from ENV must be redacted.
	if strings.Contains(debug, "EnvSecret123") {
		t.Error("debug config should not contain actual secret from ENV")
	}

	if !strings.Contains(debug, "<REDACTED>") {
		t.Error("debug config should contain redacted placeholder")
	}
}

func TestGetDebugConfigYAML_Empty(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	m := New(WithPath(tmpDir), WithEnvPrefix("MYAPP"))

	debug, err := m.GetDebugConfigYAML()
	if err != nil {
		t.Fatalf("GetDebugConfigYAML() failed: %v", err)
	}

	if !strings.Contains(debug, "No configuration sections registered") {
		t.Error("debug config should indicate no sections")
	}
}

func TestGetDebugConfigYAML_DoesNotModifyOriginal(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	m := New(WithPath(tmpDir), WithEnvPrefix("MYAPP"))

	section := &testUsageSection{}
	m.MustRegister(section)

	if err := m.Load(); err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	originalPassword := "OriginalSecret123"
	section.Password = originalPassword

	_, err := m.GetDebugConfigYAML()
	if err != nil {
		t.Fatalf("GetDebugConfigYAML() failed: %v", err)
	}

	// Original value must remain unchanged.
	if section.Password != originalPassword {
		t.Error("GetDebugConfigYAML should not modify original configuration")
	}
}

func TestGetDefaultConfigYAML_Basic(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	m := New(WithPath(tmpDir), WithEnvPrefix("MYAPP"))

	section := &testUsageSection{}
	m.MustRegister(section)

	yamlBytes, err := m.GetDefaultConfigYAML()
	if err != nil {
		t.Fatalf("GetDefaultConfigYAML() failed: %v", err)
	}

	yamlStr := string(yamlBytes)

	if !strings.Contains(yamlStr, "server:") {
		t.Error("YAML should contain server section")
	}

	if !strings.Contains(yamlStr, "port:") {
		t.Error("YAML should contain port field")
	}

	if !strings.Contains(yamlStr, "8080") {
		t.Error("YAML should contain default port value 8080")
	}

	if !strings.Contains(yamlStr, "host:") {
		t.Error("YAML should contain host field")
	}

	if !strings.Contains(yamlStr, "localhost") {
		t.Error("YAML should contain default host value")
	}

	var config map[string]any
	if err := yaml.Unmarshal(yamlBytes, &config); err != nil {
		t.Errorf("Generated YAML is not valid: %v", err)
	}
}

func TestGetDefaultConfigYAML_MultipleSections(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	m := New(WithPath(tmpDir), WithEnvPrefix("MYAPP"))

	section1 := &testUsageSection{}
	section2 := &testNestedSection{}

	m.MustRegister(section1)
	m.MustRegister(section2)

	yamlBytes, err := m.GetDefaultConfigYAML()
	if err != nil {
		t.Fatalf("GetDefaultConfigYAML() failed: %v", err)
	}

	yamlStr := string(yamlBytes)

	if !strings.Contains(yamlStr, "server:") {
		t.Error("YAML should contain server section")
	}

	if !strings.Contains(yamlStr, "nested:") {
		t.Error("YAML should contain nested section")
	}

	var config map[string]any
	if err := yaml.Unmarshal(yamlBytes, &config); err != nil {
		t.Errorf("Generated YAML is not valid: %v", err)
	}
	if _, ok := config["server"]; !ok {
		t.Error("Config should contain server key")
	}

	if _, ok := config["nested"]; !ok {
		t.Error("Config should contain nested key")
	}
}

func TestGetDefaultConfigYAML_NestedStructures(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	m := New(WithPath(tmpDir), WithEnvPrefix("MYAPP"))

	section := &testNestedSection{}
	m.MustRegister(section)

	yamlBytes, err := m.GetDefaultConfigYAML()
	if err != nil {
		t.Fatalf("GetDefaultConfigYAML() failed: %v", err)
	}

	yamlStr := string(yamlBytes)

	if !strings.Contains(yamlStr, "nested:") {
		t.Error("YAML should contain nested section")
	}

	if !strings.Contains(yamlStr, "server:") {
		t.Error("YAML should contain nested server section")
	}

	if !strings.Contains(yamlStr, "database:") {
		t.Error("YAML should contain nested database section")
	}

	// Verify default values of nested fields.
	if !strings.Contains(yamlStr, "0.0.0.0") {
		t.Error("YAML should contain default server host")
	}

	if !strings.Contains(yamlStr, "9000") {
		t.Error("YAML should contain default server port")
	}

	var config map[string]any
	if err := yaml.Unmarshal(yamlBytes, &config); err != nil {
		t.Errorf("Generated YAML is not valid: %v", err)
	}
}

func TestGetDefaultConfigYAML_DoesNotModifyOriginal(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	m := New(WithPath(tmpDir), WithEnvPrefix("MYAPP"))

	section := &testUsageSection{}
	m.MustRegister(section)

	if err := m.Load(); err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	section.Port = 9999
	section.Host = "modified"

	_, err := m.GetDefaultConfigYAML()
	if err != nil {
		t.Fatalf("GetDefaultConfigYAML() failed: %v", err)
	}

	// Original values must remain unchanged.
	if section.Port != 9999 {
		t.Error("GetDefaultConfigYAML should not modify original configuration")
	}

	if section.Host != "modified" {
		t.Error("GetDefaultConfigYAML should not modify original configuration")
	}

	// Default YAML must not reflect runtime state.
	yamlBytes, err := m.GetDefaultConfigYAML()
	if err != nil {
		t.Fatalf("GetDefaultConfigYAML() failed: %v", err)
	}

	yamlStr := string(yamlBytes)
	if strings.Contains(yamlStr, "9999") {
		t.Error("Default YAML should contain default values, not current runtime values")
	}

	if strings.Contains(yamlStr, "modified") {
		t.Error("Default YAML should contain default values, not current runtime values")
	}
}

func TestGetDefaultConfigYAML_Empty(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	m := New(WithPath(tmpDir), WithEnvPrefix("MYAPP"))

	yamlBytes, err := m.GetDefaultConfigYAML()
	if err != nil {
		t.Fatalf("GetDefaultConfigYAML() failed: %v", err)
	}

	yamlStr := string(yamlBytes)
	if !strings.Contains(yamlStr, "No configuration sections registered") {
		t.Error("YAML should indicate no sections when empty")
	}
}

func TestGetDefaultConfigYAML_TimeDuration(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	m := New(WithPath(tmpDir), WithEnvPrefix("MYAPP"))

	section := &testUsageSection{}
	m.MustRegister(section)

	yamlBytes, err := m.GetDefaultConfigYAML()
	if err != nil {
		t.Fatalf("GetDefaultConfigYAML() failed: %v", err)
	}

	yamlStr := string(yamlBytes)

	if !strings.Contains(yamlStr, "timeout:") {
		t.Error("YAML should contain timeout field")
	}
	if !strings.Contains(yamlStr, "5s") {
		t.Error("YAML should contain default timeout value as duration string")
	}
}

func TestGetDocumentation_ArrayOfStructs(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	m := New(WithPath(tmpDir), WithEnvPrefix("MYAPP"))

	section := &testArraySection{}
	m.MustRegister(section)

	docs := m.GetDocumentation()

	if len(docs) != 1 {
		t.Fatalf("expected 1 section, got %d", len(docs))
	}

	sectionDoc := docs[0]
	if sectionDoc.Key != "array" {
		t.Errorf("expected section key 'array', got %q", sectionDoc.Key)
	}

	// Must have fields for both the array and its elements.
	if len(sectionDoc.Fields) == 0 {
		t.Fatal("expected at least one field")
	}

	// First field is the array itself.
	serversField := sectionDoc.Fields[0]
	if serversField.Field != "Servers" {
		t.Errorf("expected first field to be Servers, got %q", serversField.Field)
	}

	if serversField.Type != "[]testServerConfig" {
		t.Errorf("expected Servers type '[]testServerConfig', got %q", serversField.Type)
	}

	if serversField.IsArrayElement {
		t.Error("Servers field itself should not be marked as array element")
	}

	// Verify array element fields.
	foundName := false
	foundAddress := false
	foundAliases := false

	for _, field := range sectionDoc.Fields[1:] {
		if !field.IsArrayElement {
			t.Errorf("field %q should be marked as array element", field.Field)
		}

		switch field.Field {
		case "Name":
			foundName = true
			if field.Type != "string" {
				t.Errorf("expected Name type 'string', got %q", field.Type)
			}
		case "Address":
			foundAddress = true
			if field.Type != "string" {
				t.Errorf("expected Address type 'string', got %q", field.Type)
			}
		case "Aliases":
			foundAliases = true
			if field.Type != "[]string" {
				t.Errorf("expected Aliases type '[]string', got %q", field.Type)
			}
		}
	}

	if !foundName {
		t.Error("expected to find Name field in array elements")
	}
	if !foundAddress {
		t.Error("expected to find Address field in array elements")
	}
	if !foundAliases {
		t.Error("expected to find Aliases field in array elements")
	}
}

func TestGetDefaultConfigYAML_ArrayOfStructs(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	m := New(WithPath(tmpDir), WithEnvPrefix("MYAPP"))

	section := &testArraySection{}
	m.MustRegister(section)

	yamlBytes, err := m.GetDefaultConfigYAML()
	if err != nil {
		t.Fatalf("GetDefaultConfigYAML() failed: %v", err)
	}

	yamlStr := string(yamlBytes)

	// Verify array presence.
	if !strings.Contains(yamlStr, "servers:") {
		t.Error("YAML should contain servers field")
	}

	// An empty struct array should produce an example element.
	if !strings.Contains(yamlStr, "name:") {
		t.Error("YAML should contain example server with name field")
	}

	if !strings.Contains(yamlStr, "address:") {
		t.Error("YAML should contain example server with address field")
	}

	if !strings.Contains(yamlStr, "aliases:") {
		t.Error("YAML should contain example server with aliases field")
	}

	var config map[string]any
	if err := yaml.Unmarshal(yamlBytes, &config); err != nil {
		t.Errorf("Generated YAML is not valid: %v", err)
	}

	arraySection, ok := config["array"].(map[string]any)
	if !ok {
		t.Fatal("array section should be a map")
	}

	servers, ok := arraySection["servers"].([]any)
	if !ok {
		t.Fatal("servers should be an array")
	}

	// Should have exactly one example element.
	if len(servers) != 1 {
		t.Errorf("expected 1 example server, got %d", len(servers))
	}

	if len(servers) > 0 {
		server, ok := servers[0].(map[string]any)
		if !ok {
			t.Fatal("server should be a map")
		}

		// Verify fields.
		if _, ok := server["name"]; !ok {
			t.Error("server should have name field")
		}
		if _, ok := server["address"]; !ok {
			t.Error("server should have address field")
		}
		if _, ok := server["aliases"]; !ok {
			t.Error("server should have aliases field")
		}
	}
}

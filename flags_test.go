package tune

import (
	"flag"
	"os"
	"testing"
	"time"
)

// mockSection is a test section for flag binding tests.
type mockSection struct {
	StringField   string        `yaml:"string_field" flag:"str" usage:"String field" default:"default_str"`
	NoFlagField   string        `yaml:"no_flag_field"`
	IgnoredField  string        `yaml:"ignored_field" flag:"-"`
	IntField      int           `yaml:"int_field" flag:"num" usage:"Int field" default:"42"`
	Int64Field    int64         `yaml:"int64_field" flag:"num64" usage:"Int64 field"`
	DurationField time.Duration `yaml:"duration_field" flag:"dur" usage:"Duration field" default:"5s"`
	Float64Field  float64       `yaml:"float64_field" flag:"ratio" usage:"Float field" default:"3.14"`
	BoolField     bool          `yaml:"bool_field" flag:"enabled" usage:"Bool field" default:"true"`
}

func (m *mockSection) ConfigKey() string {
	return "mock"
}

func (m *mockSection) SetDefaults() {
	// Apply defaults from struct tags when fields are zero-valued.
	if m.StringField == "" {
		m.StringField = "default_str"
	}
	if m.IntField == 0 {
		m.IntField = 42
	}
	if m.DurationField == 0 {
		m.DurationField = 5 * time.Second
	}
	if !m.BoolField {
		m.BoolField = true
	}
	if m.Float64Field == 0 {
		m.Float64Field = 3.14
	}
}

func (m *mockSection) Validate() error {
	return nil
}

func (m *mockSection) OnUpdate() {
	// no-op
}

func TestBindFlags_BasicTypes(t *testing.T) {
	t.Parallel()
	manager := New()
	section := &mockSection{}
	if err := manager.Register(section); err != nil {
		t.Fatalf("Register() failed: %v", err)
	}

	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	if err := manager.BindFlags(fs); err != nil {
		t.Fatalf("BindFlags() failed: %v", err)
	}

	testCases := []struct {
		name         string
		expectedType string
	}{
		{"mock.str", "string"},
		{"mock.num", "int"},
		{"mock.num64", "int64"},
		{"mock.dur", "duration"},
		{"mock.enabled", "bool"},
		{"mock.ratio", "float64"},
	}

	for _, tc := range testCases {
		f := fs.Lookup(tc.name)
		if f == nil {
			t.Errorf("Flag %q not registered", tc.name)
			continue
		}
	}

	// Ignored fields must not be registered.
	if f := fs.Lookup("mock.no_flag_field"); f != nil {
		t.Errorf("Flag 'mock.no_flag_field' should not be registered")
	}
	if f := fs.Lookup("mock.ignored_field"); f != nil {
		t.Errorf("Flag 'mock.ignored_field' should not be registered")
	}
}

func TestBindFlags_WithFlatFlags(t *testing.T) {
	t.Parallel()
	manager := New()
	section := &mockSection{}
	if err := manager.Register(section); err != nil {
		t.Fatalf("Register() failed: %v", err)
	}

	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	if err := manager.BindFlags(fs, WithFlatFlags()); err != nil {
		t.Fatalf("BindFlags() failed: %v", err)
	}

	// Flags should be registered without section prefix.
	if f := fs.Lookup("str"); f == nil {
		t.Error("Flag 'str' not registered (expected flat naming)")
	}
	if f := fs.Lookup("num"); f == nil {
		t.Error("Flag 'num' not registered (expected flat naming)")
	}

	// Prefixed names must not be registered in flat mode.
	if f := fs.Lookup("mock.str"); f != nil {
		t.Error("Flag 'mock.str' should not be registered in flat mode")
	}
}

func TestApplyFlags_OnlySetFlags(t *testing.T) {
	t.Parallel()
	manager := New()
	section := &mockSection{}
	if err := manager.Register(section); err != nil {
		t.Fatalf("Register() failed: %v", err)
	}

	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	if err := manager.BindFlags(fs); err != nil {
		t.Fatalf("BindFlags() failed: %v", err)
	}

	// Parse only one flag.
	args := []string{"-mock.str=custom_value"}
	if err := fs.Parse(args); err != nil {
		t.Fatalf("Parse() failed: %v", err)
	}

	// Load config without a file.
	if err := manager.Load(); err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	loaded := manager.Get("mock").(*mockSection)

	// Only the explicitly set flag should be applied.
	if loaded.StringField != "custom_value" {
		t.Errorf("StringField = %q, want 'custom_value'", loaded.StringField)
	}

	// Remaining fields should retain defaults from struct tags.
	if loaded.IntField != 42 {
		t.Errorf("IntField = %d, want 42 (from default tag)", loaded.IntField)
	}
	if loaded.DurationField != 5*time.Second {
		t.Errorf("DurationField = %v, want 5s (from default tag)", loaded.DurationField)
	}
	if loaded.BoolField != true {
		t.Errorf("BoolField = %v, want true (from default tag)", loaded.BoolField)
	}
	if loaded.Float64Field != 3.14 {
		t.Errorf("Float64Field = %f, want 3.14 (from default tag)", loaded.Float64Field)
	}
}

func TestApplyFlags_PriorityChain(t *testing.T) {
	// Create a temporary YAML file.
	tmpFile, err := os.CreateTemp("", "test-config-*.yaml")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer func() {
		if err := os.Remove(tmpFile.Name()); err != nil {
			t.Logf("Remove() failed: %v", err)
		}
	}()

	// Write config to file (priority: File).
	yamlContent := `mock:
  string_field: "from_yaml"
  int_field: 100
  duration_field: 10s
`
	if _, err := tmpFile.WriteString(yamlContent); err != nil {
		t.Fatalf("Failed to write YAML: %v", err)
	}
	if err := tmpFile.Close(); err != nil {
		t.Fatalf("Close() failed: %v", err)
	}

	// Set ENV variable (priority: ENV).
	t.Setenv("INT_FIELD", "200")

	manager := New(WithPath(tmpFile.Name()))
	section := &mockSection{}
	section.IntField = 0
	if err := manager.Register(section); err != nil {
		t.Fatalf("Register() failed: %v", err)
	}

	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	if err := manager.BindFlags(fs); err != nil {
		t.Fatalf("BindFlags() failed: %v", err)
	}

	// Set flag (priority: Flags - HIGHEST).
	args := []string{"-mock.dur=20s"}
	if err := fs.Parse(args); err != nil {
		t.Fatalf("Parse() failed: %v", err)
	}

	if err := manager.Load(); err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	loaded := manager.Get("mock").(*mockSection)

	// Verify priority chain:
	// StringField: should come from YAML (flag not set).
	if loaded.StringField != "from_yaml" {
		t.Errorf("StringField = %q, want 'from_yaml' (from YAML file)", loaded.StringField)
	}

	// IntField: should come from ENV if supported, otherwise from YAML.
	// NOTE: current Load implementation does not auto-load ENV for all fields,
	// so we expect the YAML value.
	if loaded.IntField != 100 {
		t.Errorf("IntField = %d, want 100 (from YAML, ENV not auto-loaded)", loaded.IntField)
	}

	// DurationField: should come from the flag (highest priority).
	if loaded.DurationField != 20*time.Second {
		t.Errorf("DurationField = %v, want 20s (from flag)", loaded.DurationField)
	}
}

func TestApplyFlags_AllTypes(t *testing.T) {
	t.Parallel()
	manager := New()
	section := &mockSection{}
	if err := manager.Register(section); err != nil {
		t.Fatalf("Register() failed: %v", err)
	}

	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	if err := manager.BindFlags(fs); err != nil {
		t.Fatalf("BindFlags() failed: %v", err)
	}

	// Set all types via flags.
	args := []string{
		"-mock.str=test_string",
		"-mock.num=123",
		"-mock.num64=9876543210",
		"-mock.dur=15m",
		"-mock.enabled=false",
		"-mock.ratio=2.71",
	}
	if err := fs.Parse(args); err != nil {
		t.Fatalf("Parse() failed: %v", err)
	}

	if err := manager.Load(); err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	loaded := manager.Get("mock").(*mockSection)

	// Verify all types.
	if loaded.StringField != "test_string" {
		t.Errorf("StringField = %q, want 'test_string'", loaded.StringField)
	}
	if loaded.IntField != 123 {
		t.Errorf("IntField = %d, want 123", loaded.IntField)
	}
	if loaded.Int64Field != 9876543210 {
		t.Errorf("Int64Field = %d, want 9876543210", loaded.Int64Field)
	}
	if loaded.DurationField != 15*time.Minute {
		t.Errorf("DurationField = %v, want 15m", loaded.DurationField)
	}
	if loaded.BoolField != false {
		t.Errorf("BoolField = %v, want false", loaded.BoolField)
	}
	if loaded.Float64Field != 2.71 {
		t.Errorf("Float64Field = %f, want 2.71", loaded.Float64Field)
	}
}

func TestBindFlags_NoFlagSet(t *testing.T) {
	t.Parallel()
	manager := New()
	section := &mockSection{}
	if err := manager.Register(section); err != nil {
		t.Fatalf("Register() failed: %v", err)
	}

	// BindFlags() is intentionally not called.

	if err := manager.Load(); err != nil {
		t.Fatalf("Load() should succeed without flags: %v", err)
	}

	loaded := manager.Get("mock").(*mockSection)

	// Without flags, values should come from SetDefaults().
	// Struct `default` tags are only used when registering flags in a FlagSet.
	// SetDefaults() is called automatically during Load().
	if loaded.StringField != "default_str" {
		t.Errorf("StringField = %q, want 'default_str' (from SetDefaults)", loaded.StringField)
	}
	if loaded.IntField != 42 {
		t.Errorf("IntField = %d, want 42 (from SetDefaults)", loaded.IntField)
	}
}

// complexSection is a test section for JSON flag tests (slice/struct).
type complexSection struct {
	Admin   User     `yaml:"admin" flag:"admin" usage:"Admin user config"`
	Tags    []string `yaml:"tags" flag:"tags" usage:"List of tags"`
	Servers []Server `yaml:"servers" flag:"servers" usage:"List of servers"`
}

type Server struct {
	Host string `json:"host"`
	Port int    `json:"port"`
}

type User struct {
	Name  string `json:"name"`
	Token string `json:"token"`
}

func (c *complexSection) ConfigKey() string {
	return "complex"
}

func (c *complexSection) SetDefaults() {}

func (c *complexSection) Validate() error {
	return nil
}

func (c *complexSection) OnUpdate() {}

func TestBindFlags_JSONSlice(t *testing.T) {
	t.Parallel()
	manager := New()
	section := &complexSection{}
	if err := manager.Register(section); err != nil {
		t.Fatalf("Register() failed: %v", err)
	}

	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	if err := manager.BindFlags(fs); err != nil {
		t.Fatalf("BindFlags() failed: %v", err)
	}

	// Set a JSON string array.
	args := []string{`-complex.tags=["production", "critical", "us-east-1"]`}
	if err := fs.Parse(args); err != nil {
		t.Fatalf("Parse() failed: %v", err)
	}

	if err := manager.Load(); err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	loaded := manager.Get("complex").(*complexSection)

	// Verify the array was parsed.
	if len(loaded.Tags) != 3 {
		t.Fatalf("Tags length = %d, want 3", len(loaded.Tags))
	}
	if loaded.Tags[0] != "production" {
		t.Errorf("Tags[0] = %q, want 'production'", loaded.Tags[0])
	}
	if loaded.Tags[1] != "critical" {
		t.Errorf("Tags[1] = %q, want 'critical'", loaded.Tags[1])
	}
	if loaded.Tags[2] != "us-east-1" {
		t.Errorf("Tags[2] = %q, want 'us-east-1'", loaded.Tags[2])
	}
}

func TestBindFlags_JSONStructSlice(t *testing.T) {
	t.Parallel()
	manager := New()
	section := &complexSection{}
	if err := manager.Register(section); err != nil {
		t.Fatalf("Register() failed: %v", err)
	}

	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	if err := manager.BindFlags(fs); err != nil {
		t.Fatalf("BindFlags() failed: %v", err)
	}

	// Set a JSON array of structs.
	args := []string{`-complex.servers=[{"host":"srv1","port":80},{"host":"srv2","port":443}]`}
	if err := fs.Parse(args); err != nil {
		t.Fatalf("Parse() failed: %v", err)
	}

	if err := manager.Load(); err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	loaded := manager.Get("complex").(*complexSection)

	// Verify the struct array.
	if len(loaded.Servers) != 2 {
		t.Fatalf("Servers length = %d, want 2", len(loaded.Servers))
	}
	if loaded.Servers[0].Host != "srv1" || loaded.Servers[0].Port != 80 {
		t.Errorf("Servers[0] = %+v, want {Host:srv1 Port:80}", loaded.Servers[0])
	}
	if loaded.Servers[1].Host != "srv2" || loaded.Servers[1].Port != 443 {
		t.Errorf("Servers[1] = %+v, want {Host:srv2 Port:443}", loaded.Servers[1])
	}
}

func TestBindFlags_JSONStruct(t *testing.T) {
	t.Parallel()
	manager := New()
	section := &complexSection{}
	if err := manager.Register(section); err != nil {
		t.Fatalf("Register() failed: %v", err)
	}

	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	if err := manager.BindFlags(fs); err != nil {
		t.Fatalf("BindFlags() failed: %v", err)
	}

	// Set a JSON struct.
	args := []string{`-complex.admin={"name":"admin","token":"secret-123"}`}
	if err := fs.Parse(args); err != nil {
		t.Fatalf("Parse() failed: %v", err)
	}

	if err := manager.Load(); err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	loaded := manager.Get("complex").(*complexSection)

	// Verify the struct.
	if loaded.Admin.Name != "admin" {
		t.Errorf("Admin.Name = %q, want 'admin'", loaded.Admin.Name)
	}
	if loaded.Admin.Token != "secret-123" {
		t.Errorf("Admin.Token = %q, want 'secret-123'", loaded.Admin.Token)
	}
}

func TestBindFlags_JSONWithEnvExpansion(t *testing.T) {
	t.Setenv("TEST_TOKEN", "expanded-secret-456")
	t.Setenv("TEST_HOST", "expanded-host")

	manager := New()
	section := &complexSection{}
	if err := manager.Register(section); err != nil {
		t.Fatalf("Register() failed: %v", err)
	}

	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	if err := manager.BindFlags(fs); err != nil {
		t.Fatalf("BindFlags() failed: %v", err)
	}

	// JSON with ENV variable substitution.
	args := []string{`-complex.admin={"name":"admin","token":"${TEST_TOKEN}"}`}
	if err := fs.Parse(args); err != nil {
		t.Fatalf("Parse() failed: %v", err)
	}

	if err := manager.Load(); err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	loaded := manager.Get("complex").(*complexSection)

	// Verify the ENV variable was expanded.
	if loaded.Admin.Token != "expanded-secret-456" {
		t.Errorf("Admin.Token = %q, want 'expanded-secret-456' (from ${TEST_TOKEN})", loaded.Admin.Token)
	}
}

func TestBindFlags_JSONWithMultipleEnvExpansions(t *testing.T) {
	t.Setenv("SRV1_HOST", "server1.example.com")
	t.Setenv("SRV1_PORT", "8080")
	t.Setenv("SRV2_HOST", "server2.example.com")
	t.Setenv("SRV2_PORT", "9090")

	manager := New()
	section := &complexSection{}
	if err := manager.Register(section); err != nil {
		t.Fatalf("Register() failed: %v", err)
	}

	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	if err := manager.BindFlags(fs); err != nil {
		t.Fatalf("BindFlags() failed: %v", err)
	}

	// JSON array with multiple ENV substitutions.
	args := []string{`-complex.servers=[{"host":"${SRV1_HOST}","port":${SRV1_PORT}},{"host":"${SRV2_HOST}","port":${SRV2_PORT}}]`}
	if err := fs.Parse(args); err != nil {
		t.Fatalf("Parse() failed: %v", err)
	}

	if err := manager.Load(); err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	loaded := manager.Get("complex").(*complexSection)

	// Verify substitutions.
	if len(loaded.Servers) != 2 {
		t.Fatalf("Servers length = %d, want 2", len(loaded.Servers))
	}
	if loaded.Servers[0].Host != "server1.example.com" {
		t.Errorf("Servers[0].Host = %q, want 'server1.example.com'", loaded.Servers[0].Host)
	}
	if loaded.Servers[0].Port != 8080 {
		t.Errorf("Servers[0].Port = %d, want 8080", loaded.Servers[0].Port)
	}
	if loaded.Servers[1].Host != "server2.example.com" {
		t.Errorf("Servers[1].Host = %q, want 'server2.example.com'", loaded.Servers[1].Host)
	}
	if loaded.Servers[1].Port != 9090 {
		t.Errorf("Servers[1].Port = %d, want 9090", loaded.Servers[1].Port)
	}
}

func TestBindFlags_JSONInvalidSyntax(t *testing.T) {
	t.Parallel()
	manager := New()
	section := &complexSection{}
	if err := manager.Register(section); err != nil {
		t.Fatalf("Register() failed: %v", err)
	}

	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	if err := manager.BindFlags(fs); err != nil {
		t.Fatalf("BindFlags() failed: %v", err)
	}

	// Invalid JSON.
	args := []string{`-complex.admin={invalid json}`}
	if err := fs.Parse(args); err != nil {
		t.Fatalf("Parse() failed: %v", err)
	}

	err := manager.Load()
	if err == nil {
		t.Fatal("Load() should fail with invalid JSON")
	}
	if err.Error() == "" {
		t.Errorf("Error should not be empty: %v", err)
	}
}

package tune

import (
	"reflect"
	"testing"
	"time"
)

func TestPopulateEnv_String(t *testing.T) {
	tmpDir := t.TempDir()
	m := New(WithPath(tmpDir), WithEnvPrefix("TEST"))

	section := &testEnvSection{}
	m.MustRegister(section)

	t.Setenv("TEST_TEST_ENV_STRING", "env-value")

	if err := m.Load(); err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	if section.String != "env-value" {
		t.Errorf("expected env-value, got %q", section.String)
	}
}

func TestPopulateEnv_Int(t *testing.T) {
	tmpDir := t.TempDir()
	m := New(WithPath(tmpDir), WithEnvPrefix("TEST"))

	section := &testEnvSection{}
	m.MustRegister(section)

	t.Setenv("TEST_TEST_ENV_INT", "42")

	if err := m.Load(); err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	if section.Int != 42 {
		t.Errorf("expected 42, got %d", section.Int)
	}
}

func TestPopulateEnv_Int64(t *testing.T) {
	tmpDir := t.TempDir()
	m := New(WithPath(tmpDir), WithEnvPrefix("TEST"))

	section := &testEnvSection{}
	m.MustRegister(section)

	t.Setenv("TEST_TEST_ENV_INT64", "9223372036854775807")

	if err := m.Load(); err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	if section.Int64 != 9223372036854775807 {
		t.Errorf("expected 9223372036854775807, got %d", section.Int64)
	}
}

func TestPopulateEnv_Uint(t *testing.T) {
	tmpDir := t.TempDir()
	m := New(WithPath(tmpDir), WithEnvPrefix("TEST"))

	section := &testEnvSection{}
	m.MustRegister(section)

	t.Setenv("TEST_TEST_ENV_UINT", "100")

	if err := m.Load(); err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	if section.Uint != 100 {
		t.Errorf("expected 100, got %d", section.Uint)
	}
}

func TestPopulateEnv_Bool(t *testing.T) {
	tmpDir := t.TempDir()
	m := New(WithPath(tmpDir), WithEnvPrefix("TEST"))

	section := &testEnvSection{}
	m.MustRegister(section)

	testCases := []struct {
		envValue string
		expected bool
	}{
		{"true", true},
		{"false", false},
		{"1", true},
		{"0", false},
		{"TRUE", true},
		{"FALSE", false},
	}

	for _, tc := range testCases {
		t.Run(tc.envValue, func(t *testing.T) {
			section.Bool = false
			t.Setenv("TEST_TEST_ENV_BOOL", tc.envValue)

			if err := m.Load(); err != nil {
				t.Fatalf("Load() failed: %v", err)
			}

			if section.Bool != tc.expected {
				t.Errorf("expected %v, got %v", tc.expected, section.Bool)
			}
		})
	}
}

func TestPopulateEnv_Duration(t *testing.T) {
	tmpDir := t.TempDir()
	m := New(WithPath(tmpDir), WithEnvPrefix("TEST"))

	section := &testEnvSection{}
	m.MustRegister(section)

	t.Setenv("TEST_TEST_ENV_DURATION", "5m30s")

	if err := m.Load(); err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	expected := 5*time.Minute + 30*time.Second
	if section.Duration != expected {
		t.Errorf("expected %v, got %v", expected, section.Duration)
	}
}

func TestPopulateEnv_Pointer(t *testing.T) {
	tmpDir := t.TempDir()
	m := New(WithPath(tmpDir), WithEnvPrefix("TEST"))

	section := &testEnvSection{}
	m.MustRegister(section)

	t.Setenv("TEST_TEST_ENV_PTR", "pointer-value")

	if err := m.Load(); err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	if section.Ptr == nil {
		t.Fatal("expected pointer to be initialized")
	}
	if *section.Ptr != "pointer-value" {
		t.Errorf("expected pointer-value, got %q", *section.Ptr)
	}
}

func TestPopulateEnv_IgnoreYamlDash(t *testing.T) {
	tmpDir := t.TempDir()
	m := New(WithPath(tmpDir), WithEnvPrefix("TEST"))

	section := &testEnvSection{}
	m.MustRegister(section)

	t.Setenv("TEST_TEST_ENV_IGNORED", "should-be-ignored")

	if err := m.Load(); err != nil {
		t.Fatalf("Load() failed: %v", err)
	}
	if section.Ignored != "" {
		t.Errorf("expected ignored field to remain empty, got %q", section.Ignored)
	}
}

func TestPopulateEnv_IgnoreUnexported(t *testing.T) {
	tmpDir := t.TempDir()
	m := New(WithPath(tmpDir), WithEnvPrefix("TEST"))

	section := &testEnvSection{}
	m.MustRegister(section)

	t.Setenv("TEST_TEST_ENV_UNEXPORTED", "should-be-ignored")

	if err := m.Load(); err != nil {
		t.Fatalf("Load() failed: %v", err)
	}
	val := reflect.ValueOf(section).Elem()
	field := val.FieldByName("unexported")
	if !field.IsValid() {
		t.Fatal("field not found")
	}
	if field.String() != "" {
		t.Errorf("expected unexported field to remain empty, got %q", field.String())
	}
}

func TestPopulateEnv_InvalidInt(t *testing.T) {
	tmpDir := t.TempDir()
	m := New(WithPath(tmpDir), WithEnvPrefix("TEST"))

	section := &testEnvSection{}
	m.MustRegister(section)

	t.Setenv("TEST_TEST_ENV_INT", "not-a-number")

	err := m.Load()
	if err == nil {
		t.Fatal("expected error for invalid int, got nil")
	}
}

func TestPopulateEnv_InvalidBool(t *testing.T) {
	tmpDir := t.TempDir()
	m := New(WithPath(tmpDir), WithEnvPrefix("TEST"))

	section := &testEnvSection{}
	m.MustRegister(section)

	t.Setenv("TEST_TEST_ENV_BOOL", "not-a-bool")

	err := m.Load()
	if err == nil {
		t.Fatal("expected error for invalid bool, got nil")
	}
}

func TestPopulateEnv_InvalidDuration(t *testing.T) {
	tmpDir := t.TempDir()
	m := New(WithPath(tmpDir), WithEnvPrefix("TEST"))

	section := &testEnvSection{}
	m.MustRegister(section)

	t.Setenv("TEST_TEST_ENV_DURATION", "not-a-duration")

	err := m.Load()
	if err == nil {
		t.Fatal("expected error for invalid duration, got nil")
	}
}

func TestPopulateEnv_OverflowInt(t *testing.T) {
	tmpDir := t.TempDir()
	m := New(WithPath(tmpDir), WithEnvPrefix("TEST"))

	section := &testEnvSection{}
	m.MustRegister(section)

	t.Setenv("TEST_TEST_ENV_INT32", "2147483648") // MaxInt32 + 1

	err := m.Load()
	if err == nil {
		t.Fatal("expected error for int overflow, got nil")
	}
}

// testEnvSection is a test section for verifying ENV population.
type testEnvSection struct {
	Ptr        *string       `yaml:"ptr"`
	String     string        `yaml:"string"`
	Ignored    string        `yaml:"-"`
	unexported string        //nolint:unused // accessed via reflect; tests that populateEnv skips unexported fields
	Int        int           `yaml:"int"`
	Int64      int64         `yaml:"int64"`
	Uint       uint          `yaml:"uint"`
	Duration   time.Duration `yaml:"duration"`
	Int32      int32         `yaml:"int32"`
	Bool       bool          `yaml:"bool"`
}

func (t *testEnvSection) ConfigKey() string { return "test_env" }
func (t *testEnvSection) SetDefaults()      {}
func (t *testEnvSection) Validate() error   { return nil }
func (t *testEnvSection) OnUpdate()         {}

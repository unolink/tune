package tune

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// benchSection is a config section used for performance benchmarks.
type benchSection struct {
	Host        string        `yaml:"host"`
	LogLevel    string        `yaml:"log_level"`
	DatabaseURL string        `yaml:"database_url" secret:"true"`
	APIKey      string        `yaml:"api_key" secret:"true"`
	Port        int           `yaml:"port"`
	Timeout     time.Duration `yaml:"timeout"`
	MaxConns    int           `yaml:"max_conns"`
	Enabled     bool          `yaml:"enabled"`
}

func (b *benchSection) ConfigKey() string { return "bench" }
func (b *benchSection) SetDefaults() {
	b.Port = 8080
	b.Host = "localhost"
	b.Timeout = 5 * time.Second
	b.Enabled = true
	b.MaxConns = 100
	b.LogLevel = "info"
	b.DatabaseURL = "postgres://localhost/db"
	b.APIKey = "default-api-key"
}
func (b *benchSection) Validate() error { return nil }
func (b *benchSection) OnUpdate()       {}

// benchSetenv sets environment variables for benchmarks.
// testing.B does not have Setenv, so we use os.Setenv directly.
func benchSetenv(b *testing.B, key, value string) {
	b.Helper()
	if err := os.Setenv(key, value); err != nil {
		b.Fatalf("Setenv(%q) failed: %v", key, err)
	}
}

// benchUnsetenv removes environment variables after benchmarks.
func benchUnsetenv(b *testing.B, keys ...string) {
	b.Helper()
	for _, key := range keys {
		if err := os.Unsetenv(key); err != nil {
			b.Fatalf("Unsetenv(%q) failed: %v", key, err)
		}
	}
}

// BenchmarkLoad measures the full Load() cycle performance.
func BenchmarkLoad(b *testing.B) {
	tmpDir := b.TempDir()

	yamlContent := `bench:
  port: 9000
  host: "0.0.0.0"
  timeout: "10s"
  enabled: true
  max_conns: 200
  log_level: "debug"
  database_url: "postgres://user:pass@localhost/db"
  api_key: "test-api-key"
`
	configFile := filepath.Join(tmpDir, "config.yml")
	if err := os.WriteFile(configFile, []byte(yamlContent), 0o644); err != nil {
		b.Fatalf("failed to write test config: %v", err)
	}

	m := New(WithPath(tmpDir), WithEnvPrefix("BENCH"))
	section := &benchSection{}
	m.MustRegister(section)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := m.Load(); err != nil {
			b.Fatalf("Load() failed: %v", err)
		}
	}
}

// BenchmarkLoadWithENV measures Load() performance with ENV variables.
func BenchmarkLoadWithENV(b *testing.B) {
	tmpDir := b.TempDir()

	yamlContent := `bench:
  port: 9000
  host: "0.0.0.0"
`
	configFile := filepath.Join(tmpDir, "config.yml")
	if err := os.WriteFile(configFile, []byte(yamlContent), 0o644); err != nil {
		b.Fatalf("failed to write test config: %v", err)
	}

	envKeys := []string{"BENCH_BENCH_PORT", "BENCH_BENCH_HOST", "BENCH_BENCH_TIMEOUT", "BENCH_BENCH_ENABLED", "BENCH_BENCH_MAXCONNS"}
	benchSetenv(b, "BENCH_BENCH_PORT", "7777")
	benchSetenv(b, "BENCH_BENCH_HOST", "127.0.0.1")
	benchSetenv(b, "BENCH_BENCH_TIMEOUT", "20s")
	benchSetenv(b, "BENCH_BENCH_ENABLED", "true")
	benchSetenv(b, "BENCH_BENCH_MAXCONNS", "500")
	defer benchUnsetenv(b, envKeys...)

	m := New(WithPath(tmpDir), WithEnvPrefix("BENCH"))
	section := &benchSection{}
	m.MustRegister(section)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := m.Load(); err != nil {
			b.Fatalf("Load() failed: %v", err)
		}
	}
}

// BenchmarkPopulateEnv measures ENV variable application performance.
func BenchmarkPopulateEnv(b *testing.B) {
	tmpDir := b.TempDir()
	m := New(WithPath(tmpDir), WithEnvPrefix("BENCH"))

	envKeys := []string{"BENCH_BENCH_PORT", "BENCH_BENCH_HOST", "BENCH_BENCH_TIMEOUT", "BENCH_BENCH_ENABLED", "BENCH_BENCH_MAXCONNS"}
	benchSetenv(b, "BENCH_BENCH_PORT", "7777")
	benchSetenv(b, "BENCH_BENCH_HOST", "127.0.0.1")
	benchSetenv(b, "BENCH_BENCH_TIMEOUT", "20s")
	benchSetenv(b, "BENCH_BENCH_ENABLED", "true")
	benchSetenv(b, "BENCH_BENCH_MAXCONNS", "500")
	defer benchUnsetenv(b, envKeys...)

	section := &benchSection{}
	section.SetDefaults()
	m.MustRegister(section)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := m.populateEnv(section); err != nil {
			b.Fatalf("populateEnv() failed: %v", err)
		}
	}
}

// BenchmarkExpandEnv measures ENV variable expansion performance.
func BenchmarkExpandEnv(b *testing.B) {
	tmpDir := b.TempDir()
	m := New(WithPath(tmpDir), WithEnvPrefix("BENCH"))

	envKeys := []string{"DB_HOST", "DB_PORT", "DB_USER", "DB_PASS"}
	benchSetenv(b, "DB_HOST", "production-db.example.com")
	benchSetenv(b, "DB_PORT", "5432")
	benchSetenv(b, "DB_USER", "admin")
	benchSetenv(b, "DB_PASS", "secret123")
	defer benchUnsetenv(b, envKeys...)

	section := &benchSection{}
	section.SetDefaults()
	section.DatabaseURL = "postgres://${DB_USER}:${DB_PASS}@${DB_HOST}:${DB_PORT}/mydb"
	section.Host = "${DB_HOST}"

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := m.expandEnv(section); err != nil {
			b.Fatalf("expandEnv() failed: %v", err)
		}
	}
}

// BenchmarkAnalyzeSection measures analyzeSection performance.
func BenchmarkAnalyzeSection(b *testing.B) {
	tmpDir := b.TempDir()
	m := New(WithPath(tmpDir), WithEnvPrefix("BENCH"))

	section := &benchSection{}
	m.MustRegister(section)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = m.analyzeSection(section)
	}
}

// BenchmarkGetDocumentation measures GetDocumentation performance.
func BenchmarkGetDocumentation(b *testing.B) {
	tmpDir := b.TempDir()
	m := New(WithPath(tmpDir), WithEnvPrefix("BENCH"))

	section := &benchSection{}
	m.MustRegister(section)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = m.GetDocumentation()
	}
}

// BenchmarkGetDebugConfigYAML measures GetDebugConfigYAML performance.
func BenchmarkGetDebugConfigYAML(b *testing.B) {
	tmpDir := b.TempDir()
	m := New(WithPath(tmpDir), WithEnvPrefix("BENCH"))

	section := &benchSection{}
	m.MustRegister(section)

	if err := m.Load(); err != nil {
		b.Fatalf("Load() failed: %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := m.GetDebugConfigYAML()
		if err != nil {
			b.Fatalf("GetDebugConfigYAML() failed: %v", err)
		}
	}
}

// BenchmarkGetDefaultConfigYAML measures GetDefaultConfigYAML performance.
func BenchmarkGetDefaultConfigYAML(b *testing.B) {
	tmpDir := b.TempDir()
	m := New(WithPath(tmpDir), WithEnvPrefix("BENCH"))

	section := &benchSection{}
	m.MustRegister(section)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := m.GetDefaultConfigYAML()
		if err != nil {
			b.Fatalf("GetDefaultConfigYAML() failed: %v", err)
		}
	}
}

// BenchmarkCopySections measures copySections performance.
func BenchmarkCopySections(b *testing.B) {
	tmpDir := b.TempDir()
	m := New(WithPath(tmpDir), WithEnvPrefix("BENCH"))

	section := &benchSection{}
	m.MustRegister(section)

	if err := m.Load(); err != nil {
		b.Fatalf("Load() failed: %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = m.copySections()
	}
}

// BenchmarkDiff measures Diff performance.
func BenchmarkDiff(b *testing.B) {
	oldSection := &benchSection{}
	oldSection.SetDefaults()

	newSection := &benchSection{}
	newSection.SetDefaults()
	newSection.Port = 9999
	newSection.Host = "0.0.0.0"
	newSection.MaxConns = 500

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Diff(oldSection, newSection)
	}
}

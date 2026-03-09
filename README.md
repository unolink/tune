[Русская версия (README.ru.md)](README.ru.md)

# tune

Struct-first configuration for Go: YAML, ENV, CLI flags, hot-reload — from a single struct definition.

## Why tune

Most config libraries make you choose: either you manually map ENV vars, or you write boilerplate to bridge YAML and flags, or you give up on hot-reload. tune does all of it from one Go struct with zero ceremony.

- **One struct, all sources** — define fields once, get YAML, ENV, and CLI flag support automatically
- **Strict priority chain** — Defaults → YAML → ENV → Flags, always predictable
- **Hot-reload** — file watcher with Copy-on-Write, no restart needed
- **${VAR} expansion** — environment variables inside string fields, including nested structs and JSON
- **Secret masking** — `secret:"true"` tag, debug dump shows `<REDACTED>`
- **Field locking** — know exactly which source set each field (yaml / env / flag)
- **Auto-generated docs** — CLI usage and default YAML template from struct tags
- **One dependency** — only `gopkg.in/yaml.v3`

## Install

```bash
go get github.com/unolink/tune
```

## Quick Start

Define your config as a struct implementing `tune.Section`:

```go
type ServerConfig struct {
    Host    string        `yaml:"host"`
    Port    int           `yaml:"port"`
    Timeout time.Duration `yaml:"timeout"`
    Secret  string        `yaml:"secret" secret:"true"`
}

func (c *ServerConfig) ConfigKey() string { return "server" }
func (c *ServerConfig) SetDefaults() {
    c.Host = "localhost"
    c.Port = 8080
    c.Timeout = 30 * time.Second
}
func (c *ServerConfig) Validate() error {
    if c.Port <= 0 {
        return fmt.Errorf("port must be positive")
    }
    return nil
}
func (c *ServerConfig) OnUpdate() {
    // Called on hot-reload when this section changes.
}
```

Load configuration:

```go
m := tune.New(
    tune.WithPath("/etc/myapp/config.yml"),
    tune.WithEnvPrefix("MYAPP"),
)
cfg := &ServerConfig{}
m.MustRegister(cfg)

if err := m.Load(); err != nil {
    log.Fatal(err)
}

fmt.Println(cfg.Host, cfg.Port) // values from YAML, overridden by ENV, overridden by flags
```

## Priority Chain

Each field is resolved in strict order. Later sources win:

```
Defaults → YAML file → ENV variables → CLI flags
```

Given this YAML:
```yaml
server:
  host: "0.0.0.0"
  port: 9000
```

And this ENV:
```bash
export MYAPP_SERVER_PORT=3000
```

The result is `host="0.0.0.0"` (from YAML), `port=3000` (ENV overrides YAML).

## ENV Variables

ENV names are derived automatically: `UPPER(prefix + "_" + section_key + "_" + field_name)`.

| Struct Field | ENV Variable |
|---|---|
| `Host` | `MYAPP_SERVER_HOST` |
| `Port` | `MYAPP_SERVER_PORT` |
| `Timeout` | `MYAPP_SERVER_TIMEOUT` |

Supported types: `string`, `int*`, `uint*`, `float*`, `bool`, `time.Duration`, pointers, slices and structs (via JSON).

## ENV Expansion

String fields support `${VAR}` substitution:

```yaml
server:
  host: "${DB_HOST}"
  secret: "postgres://${DB_USER}:${DB_PASS}@${DB_HOST}/mydb"
```

Works in YAML values, ENV values, and JSON flag values.

## CLI Flags

Add `flag` tags to bind fields to CLI flags:

```go
type ServerConfig struct {
    Host string `yaml:"host" flag:"host" usage:"Address to bind" default:"localhost"`
    Port int    `yaml:"port" flag:"port" usage:"HTTP port" default:"8080"`
}
```

Bind to a `flag.FlagSet`:

```go
m := tune.New(tune.WithEnvPrefix("MYAPP"))
m.MustRegister(&ServerConfig{})

fs := flag.NewFlagSet("app", flag.ExitOnError)
m.BindFlags(fs)            // flags: --server.host, --server.port
// or
m.BindFlags(fs, tune.WithFlatFlags()) // flags: --host, --port

fs.Parse(os.Args[1:])
m.Load() // flags override ENV and YAML
```

Slices and structs accept JSON via flags:

```bash
./app --server.tags='["prod","us-east"]' --server.admin='{"name":"root","token":"${SECRET}"}'
```

## Hot-Reload

```go
m.Load()
m.Watch(5 * time.Second) // polls file ModTime every 5s
defer m.StopWatch()
```

When the file changes:
1. New values are loaded without holding the lock (Copy-on-Write)
2. Changed sections receive `OnUpdate()` calls
3. Direct references (`cfg.Port`) reflect new values — no pointer indirection needed

Config readers are never blocked by the reload process.

## Secret Masking

```go
type DBConfig struct {
    Host     string `yaml:"host"`
    Password string `yaml:"password" secret:"true"`
}
```

```go
yaml, _ := m.GetDebugConfigYAML()
fmt.Println(yaml)
// db:
//   host: localhost
//   password: <REDACTED>
```

## Field Locking

After `Load()`, inspect which source set each field:

```go
locks := m.LockedFields("server")
// map[string]string{"host": "yaml", "port": "env", "timeout": "flag"}
```

Useful for UI indicators, audit logs, or preventing runtime overrides of externally-managed fields.

## Documentation Generation

### CLI Usage

```go
fmt.Println(m.GetUsage())
```

```
SECTION: server (ENV Prefix: MYAPP_SERVER_)
--------------------------------------------------------------------------------
Field:        Host
Type:         string
ENV:          MYAPP_SERVER_HOST
YAML:         host
Default:      "localhost"
Usage:        Address to bind
```

### Default Config Template

```go
yaml, _ := m.GetDefaultConfigYAML()
os.WriteFile("config.default.yml", yaml, 0o644)
```

Generates a valid YAML file populated with all default values — ready to use as a starting template.

### Structured Documentation

```go
docs := m.GetDocumentation() // []DocSection with typed field metadata
```

Returns machine-readable metadata for building custom help pages, web UIs, or API responses.

## Directory Mode

Point to a directory to merge multiple YAML files:

```go
m := tune.New(tune.WithPath("/etc/myapp/config.d"))
```

All `.yml`/`.yaml` files are merged in alphabetical order. Later files override earlier ones on the same top-level key.

## Diff

Compare two section snapshots:

```go
changes := tune.Diff(oldCfg, newCfg)
// ["port: 8080 -> 9000", "host: \"localhost\" -> \"0.0.0.0\"", "password: changed"]
```

Secret fields report `changed` without exposing values.

## Dependencies

- `gopkg.in/yaml.v3` — YAML parsing

## License

MIT — see [LICENSE](LICENSE).

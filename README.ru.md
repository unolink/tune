[English version (README.md)](README.md)

# tune

Конфигурация для Go через структуры: YAML, ENV, CLI-флаги, hot-reload — из одного определения структуры.

## Зачем tune

Большинство конфиг-библиотек заставляют выбирать: либо руками маппить ENV, либо писать boilerplate для связки YAML с флагами, либо забыть про hot-reload. tune делает всё из одной Go-структуры без лишнего кода.

- **Одна структура — все источники** — определяешь поля один раз, получаешь YAML, ENV и CLI-флаги автоматически
- **Строгий порядок приоритетов** — Defaults -> YAML -> ENV -> Flags, всегда предсказуемый
- **Hot-reload** — отслеживание файлов с Copy-on-Write, без перезапуска сервиса
- **${VAR} подстановка** — переменные окружения внутри строковых полей, включая вложенные структуры и JSON
- **Маскировка секретов** — тег `secret:"true"`, дебаг-дамп показывает `<REDACTED>`
- **Блокировка полей** — точно известно, какой источник установил каждое поле (yaml / env / flag)
- **Автогенерация документации** — CLI-справка и шаблон YAML из тегов структуры
- **Одна зависимость** — только `gopkg.in/yaml.v3`

## Установка

```bash
go get github.com/unolink/tune
```

## Быстрый старт

Определи конфигурацию как структуру, реализующую `tune.Section`:

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
    // Вызывается при hot-reload, когда секция изменилась.
}
```

Загрузка конфигурации:

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

fmt.Println(cfg.Host, cfg.Port) // значения из YAML, перезаписанные ENV, перезаписанные флагами
```

## Порядок приоритетов

Каждое поле вычисляется в строгом порядке. Более поздний источник побеждает:

```
Defaults -> YAML -> ENV -> Flags
```

При таком YAML:
```yaml
server:
  host: "0.0.0.0"
  port: 9000
```

И такой переменной окружения:
```bash
export MYAPP_SERVER_PORT=3000
```

Результат: `host="0.0.0.0"` (из YAML), `port=3000` (ENV перезаписывает YAML).

## ENV-переменные

Имена ENV формируются автоматически: `UPPER(prefix + "_" + section_key + "_" + field_name)`.

| Поле структуры | ENV-переменная |
|---|---|
| `Host` | `MYAPP_SERVER_HOST` |
| `Port` | `MYAPP_SERVER_PORT` |
| `Timeout` | `MYAPP_SERVER_TIMEOUT` |

Поддерживаемые типы: `string`, `int*`, `uint*`, `float*`, `bool`, `time.Duration`, указатели, слайсы и структуры (через JSON).

## Подстановка ENV

Строковые поля поддерживают подстановку `${VAR}`:

```yaml
server:
  host: "${DB_HOST}"
  secret: "postgres://${DB_USER}:${DB_PASS}@${DB_HOST}/mydb"
```

Работает в значениях YAML, ENV и JSON-флагах.

## CLI-флаги

Добавь теги `flag` для привязки полей к CLI-флагам:

```go
type ServerConfig struct {
    Host string `yaml:"host" flag:"host" usage:"Address to bind" default:"localhost"`
    Port int    `yaml:"port" flag:"port" usage:"HTTP port" default:"8080"`
}
```

Привязка к `flag.FlagSet`:

```go
m := tune.New(tune.WithEnvPrefix("MYAPP"))
m.MustRegister(&ServerConfig{})

fs := flag.NewFlagSet("app", flag.ExitOnError)
m.BindFlags(fs)            // флаги: --server.host, --server.port
// или
m.BindFlags(fs, tune.WithFlatFlags()) // флаги: --host, --port

fs.Parse(os.Args[1:])
m.Load() // флаги перезаписывают ENV и YAML
```

Слайсы и структуры принимают JSON через флаги:

```bash
./app --server.tags='["prod","us-east"]' --server.admin='{"name":"root","token":"${SECRET}"}'
```

## Hot-Reload

```go
m.Load()
m.Watch(5 * time.Second) // проверяет ModTime файла каждые 5 сек
defer m.StopWatch()
```

При изменении файла:
1. Новые значения загружаются без удержания блокировки (Copy-on-Write)
2. Изменённые секции получают вызов `OnUpdate()`
3. Прямые ссылки (`cfg.Port`) отражают новые значения — без косвенности через указатели

Читатели конфигурации никогда не блокируются процессом перезагрузки.

## Маскировка секретов

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

## Блокировка полей

После `Load()` можно узнать, какой источник установил каждое поле:

```go
locks := m.LockedFields("server")
// map[string]string{"host": "yaml", "port": "env", "timeout": "flag"}
```

Полезно для индикаторов в UI, аудит-логов или запрета runtime-перезаписи полей, управляемых извне.

## Генерация документации

### CLI-справка

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

### Шаблон конфига по умолчанию

```go
yaml, _ := m.GetDefaultConfigYAML()
os.WriteFile("config.default.yml", yaml, 0o644)
```

Генерирует валидный YAML-файл со всеми значениями по умолчанию — готовый стартовый шаблон.

### Структурированная документация

```go
docs := m.GetDocumentation() // []DocSection с типизированными метаданными полей
```

Возвращает машиночитаемые метаданные для построения собственных страниц помощи, веб-интерфейсов или API-ответов.

## Режим директории

Укажи директорию для объединения нескольких YAML-файлов:

```go
m := tune.New(tune.WithPath("/etc/myapp/config.d"))
```

Все файлы `.yml`/`.yaml` объединяются в алфавитном порядке. Более поздние файлы перезаписывают предыдущие по одинаковому ключу верхнего уровня.

## Сравнение конфигураций

```go
changes := tune.Diff(oldCfg, newCfg)
// ["port: 8080 -> 9000", "host: \"localhost\" -> \"0.0.0.0\"", "password: changed"]
```

Секретные поля показывают `changed` без раскрытия значений.

## Зависимости

- `gopkg.in/yaml.v3` — парсинг YAML

## Лицензия

MIT — см. [LICENSE](LICENSE).

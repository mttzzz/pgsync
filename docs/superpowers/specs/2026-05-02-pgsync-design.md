# pgsync — Design Spec

**Дата:** 2026-05-02
**Автор:** Kiril (taborrnd@gmail.com), assisted by Claude
**Статус:** Approved (brainstorming → spec)

---

## 1. Назначение

CLI/TUI-инструмент на Go, аналог существующего `dbsync` (для MySQL), но для PostgreSQL.

Основной user story: *«как разработчик, я хочу одной командой / парой кликов в TUI подтянуть прод-БД на свой локальный PostgreSQL для разработки и тестов, через корпоративный прокси, максимально быстро».*

Целевые платформы: **Windows x64**, **macOS arm64**, **macOS amd64**, **Linux amd64** (все four — single static binary).

---

## 2. Цели и не-цели

### Цели (MVP)
- Pull всей prod-БД или подмножества таблиц (с авто-добавлением FK-зависимостей) на локальный PG.
- Скорость в порядке `pgcopydb`-класса (parallel COPY BINARY, parallel index build).
- **Single binary без внешних установок у конечного пользователя** — `pg_dump`/`pg_restore` эмбеддятся в наш бинарь под каждую платформу.
- Поддержка прокси: SOCKS5 / SOCKS5h / HTTP / HTTPS (как в dbsync).
- Два UX-режима: full-screen TUI (default при запуске без аргументов) и CLI-команды.
- LLM-friendly режим: `--output=json` → NDJSON-stream событий (для агентного запуска).
- Конфиг **полностью управляется через TUI**; пользователь никогда руками не правит файл (это деталь хранения, не интерфейс).
- Self-update: команда `pgsync upgrade`.
- Тестирование на трёх уровнях (unit, integration, e2e) — обязательное правило проекта.

### Не-цели (out of scope для MVP)
- Push (local → remote) или двусторонняя синхронизация. Только pull.
- Continuous replication / change-data-capture (это домен `pgcopydb follow`, нам не нужно).
- Cross-engine миграция (MySQL → PG) — это была разовая задача, есть отдельный bun-скрипт.
- Поддержка PG <18 — таргетим **18+ как minimum** (latest stable, релиз сентября 2025). И source, и target должны быть PG 18+. Старые версии — out of scope; если понадобится, открываем отдельный issue.
- Linux ARM64, FreeBSD и прочее — добавим если попросят.
- Web-UI / GUI вне терминала.

---

## 3. Бэкенд: «pgcopydb-lite» на pgx

### 3.1 Почему не pgcopydb / pg_dump+pg_restore напрямую

Рассмотрены три варианта:

- **A) Wrap `pgcopydb`** — отлично работает на macOS/Linux, но **нет нативного Windows-бинаря** (issue #80, закрыт мейнтейнером без resolution; *«contributions welcome»*). Это пожизненный блокер для Windows-пользователей.
- **B) Wrap `pg_dump --format=directory --jobs=N` + `pg_restore --jobs=N`** — кросс-платформенно, проще, но пишет на диск (двойной I/O), perf просядет на больших БД.
- **C) Native pipeline на pgx + делегирование DDL в pg_dump** ← **выбрано**.

### 3.2 Пайплайн (вариант C)

`pgcopydb` сам внутри делегирует DDL в `pg_dump`, а параллелизирует только данные и индексы. Делаем то же самое:

```
[1] BEGIN ISOLATION LEVEL REPEATABLE READ; SELECT pg_export_snapshot();
    → snapshot_id для consistency между параллельными воркерами
[2] pg_dump --section=pre-data --schema-only --no-owner --no-acl
    → DDL: CREATE SCHEMA, CREATE SEQUENCE, CREATE TYPE, CREATE TABLE без INDEX/FK
[3] На target: DROP DATABASE/SCHEMA CASCADE → CREATE → applies pre-data DDL
[4] Параллельно (N воркеров, дефолт runtime.NumCPU()):
    for table in topological_order(tables):
        SET TRANSACTION SNAPSHOT '<snapshot_id>';
        pgx.CopyTo(source, "COPY <t> TO STDOUT WITH (FORMAT BINARY)") →
        pgx.CopyFrom(target, "COPY <t> FROM STDIN WITH (FORMAT BINARY)")
        Прогресс через io.Reader-обёртку, считающую байты и rows.
[5] pg_dump --section=post-data --no-owner --no-acl
    → INDEX, CONSTRAINT (FK/PK), TRIGGER. Применяем на target.
    Можно параллельно через psql -c с пулом, или просто apply последовательно
    (CREATE INDEX CONCURRENTLY если флаг включён).
[6] Для каждого sequence: SELECT setval('<seq>', (SELECT max(<col>) FROM <table>));
[7] (Опционально) ANALYZE на target.
```

### 3.3 Engine abstraction

```go
type Engine interface {
    Plan(ctx context.Context, opts PlanOptions) (*Plan, error)
    Execute(ctx context.Context, plan *Plan, observer ProgressObserver) (*Result, error)
}

// Реализации:
type NativeEngine struct { /* pgx + embedded pg_dump/pg_restore */ }
type ExternalEngine struct { /* shells out to system pgcopydb */ }
```

`NativeEngine` — main-path. `ExternalEngine` — опциональный fallback для пользователей, у которых уже стоит pgcopydb (флаг `--engine=external` или авто-выбор если в `$PATH`).

---

## 4. Embedded pg_dump / pg_restore

### 4.1 Почему эмбеддим

- Single-binary UX без зависимостей.
- Гарантируем известную/протестированную версию pg_dump (избегаем «у юзера PG 11, прод PG 17, dump несовместим»).
- Юзер всё ещё может попросить системную версию через `--use-system-pgtools`.

### 4.2 Что эмбеддим (per platform)

Версия pg_tools = **18** (matching minimum supported PG).

| Platform | Files | Приблизительный размер |
|---|---|---|
| windows-amd64 | `pg_dump.exe`, `pg_restore.exe`, `libpq.dll`, `libintl-*.dll`, `libwinpthread-1.dll`, `libcrypto-3-x64.dll`, `libssl-3-x64.dll`, `libiconv-2.dll` | ~25 MB |
| darwin-arm64 | `pg_dump`, `pg_restore`, `libpq.5.dylib` (+ rpath fix) | ~20 MB |
| darwin-amd64 | то же | ~20 MB |
| linux-amd64 | `pg_dump`, `pg_restore`, `libpq.so.5` | ~15 MB |

Источник: официальные `postgresql-18.x-<platform>-binaries.zip` с postgresql.org / EDB downloads / homebrew bottles. Проверяем sha256 при сборке (`scripts/verify-pgtools.sh`).

### 4.3 Build-time embedding

```go
// internal/engine/pgtools/embed_windows_amd64.go
//go:build windows && amd64
//go:embed bin/windows-amd64/*
var embeddedTools embed.FS
```

Аналогично для других платформ. Build tag-based — каждый бинарь несёт только свою платформу.

### 4.4 Runtime extraction

При первом запуске:
1. Считаем sha256 эмбедов и сравниваем с `~/.pgsync/cache/<sha>/.installed`.
2. Если не извлечено — распаковываем в `~/.pgsync/cache/<sha>/bin/` (Win: `%LOCALAPPDATA%\pgsync\cache\`).
3. На macOS — `chmod +x` + при необходимости `codesign --sign - --identifier dev.pgsync.bundled` (как dbsync делает).
4. Возвращаем абсолютные пути, передаём в `os/exec.Command`.

Кэшируем по хешу версии — ребилд pgsync с обновлёнными pg_tools → новая папка кэша, старая не мешает.

### 4.5 Альтернатива: download-on-first-run

Если бинарь 30 MB неприемлем, можно вместо embed качать с GitHub Releases при первом запуске (`pgsync setup`). **Решение для MVP: эмбеддим.** Юзер платит 30 MB один раз, получает offline-friendly опыт. Download-режим — фича, добавим если попросят.

---

## 5. Layout проекта

```
pgsync/
├── cmd/pgsync/
│   └── main.go
├── internal/
│   ├── cli/
│   │   ├── commands.go              # cobra root + subcommands
│   │   ├── sync.go                  # sync command
│   │   ├── list.go, status.go, doctor.go, config.go, upgrade.go
│   │   ├── plain_output.go          # human-friendly stdout
│   │   ├── agent_progress.go        # NDJSON для --output=json
│   │   └── interactive.go, text_interactive.go
│   ├── tui/
│   │   ├── app.go                   # bubbletea App
│   │   ├── screens/                 # settings, dbs, tables, confirm, progress, result
│   │   └── styles/                  # lipgloss styles
│   ├── engine/
│   │   ├── engine.go                # Engine interface + Plan / Result / ProgressObserver
│   │   ├── native/
│   │   │   ├── native.go            # NativeEngine
│   │   │   ├── snapshot.go          # snapshot export + transaction setup
│   │   │   ├── copy.go              # parallel COPY BINARY pipeline
│   │   │   ├── schema.go            # pre-data / post-data apply
│   │   │   └── sequences.go
│   │   ├── external/
│   │   │   └── pgcopydb.go          # обёртка над system pgcopydb (опционально)
│   │   └── pgtools/
│   │       ├── embed_<platform>.go  # //go:embed per platform
│   │       ├── extract.go           # runtime extraction в кэш
│   │       └── locate.go            # find embedded vs system
│   ├── pgschema/
│   │   ├── parser.go                # парсинг pg_dump output (split pre/post-data)
│   │   ├── tables.go                # ListTables, размеры, owner
│   │   ├── deps.go                  # FK-граф, топологическая сортировка
│   │   └── filter.go                # --tables filter с auto-FK closure
│   ├── proxy/
│   │   ├── tunnel.go                # TCP-туннель через SOCKS5/HTTP (порт из dbsync)
│   │   └── dialer.go
│   ├── config/
│   │   ├── config.go                # struct + load/save TOML
│   │   ├── store.go                 # atomic write (tmp → rename), 0600 perms
│   │   ├── path.go                  # XDG / APPDATA resolver
│   │   ├── override.go              # env + CLI overrides (read-only, не пишут в файл)
│   │   └── validate.go              # валидаторы полей для TUI inline-validation
│   ├── models/
│   │   ├── plan.go, result.go, progress.go
│   │   └── table.go, database.go
│   ├── observability/
│   │   ├── logger.go                # slog setup, text/JSON handlers
│   │   └── progress.go              # ProgressObserver реализации
│   ├── runner/
│   │   ├── runner.go                # CommandRunner / StreamingRunner интерфейсы
│   │   ├── exec.go                  # production реализация (os/exec)
│   │   └── fake_test.go             # хелперы для unit-tests
│   ├── clock/
│   │   ├── clock.go                 # Clock interface
│   │   └── system.go                # time.Now() реализация
│   ├── fsx/
│   │   ├── fs.go                    # FS interface (read/write/stat/rename)
│   │   └── os.go                    # production реализация
│   ├── version/
│   │   └── version.go
│   └── updater/
│       └── updater.go               # порт из dbsync
├── pkg/utils/
├── test/
│   ├── helpers/                     # общие test helpers (для всех уровней)
│   ├── integration/
│   │   ├── pipeline_test.go         # testcontainers: 2 PG, full sync, row count + md5
│   │   ├── partial_test.go          # --tables + auto-FK
│   │   ├── proxy_test.go            # SOCKS5 контейнер
│   │   └── interrupt_test.go        # SIGINT mid-copy
│   └── e2e/
│       └── real_db_test.go          # manual / workflow_dispatch против прод-БД
├── benchmarks/
│   ├── runner_test.go               # testing.B harness
│   ├── compare.go                   # diff двух прогонов
│   └── results/                     # <git-sha>/<fixture>.json + HISTORY.md
├── fixtures/
│   ├── genfixture/                  # cmd для pgsync-genfixture
│   ├── upload-prod.sh               # one-shot заливает фикстуры на твой прод
│   ├── dvdrental.sql.gz             # публичная small-фикстура (downloader)
│   └── README.md
├── scripts/
│   ├── fetch-pgtools.sh             # скачивает pg_dump/pg_restore с postgresql.org для embed/
│   └── verify-pgtools.sh            # checksums
├── embed/bin/
│   ├── windows-amd64/...
│   ├── darwin-arm64/...
│   ├── darwin-amd64/...
│   └── linux-amd64/...
├── docs/
│   ├── superpowers/specs/
│   │   └── 2026-05-02-pgsync-design.md  ← этот файл
│   └── README.md
├── .github/workflows/                # CI: build all, test, release on tag
├── go.mod, go.sum
├── Makefile, build.ps1
├── .env.example
├── README.md, CHANGELOG.md, LICENSE
└── .gitignore
```

---

## 6. CLI surface

```bash
pgsync                          # → TUI
pgsync tui                      # explicit TUI
pgsync text                     # text-mode interactive (no full-screen)
pgsync list                     # list databases on remote
pgsync status                   # check connections (incl. proxy)
pgsync sync <db>                # one-shot pull, with confirmation
pgsync sync <db> --tables a,b,c # subset, auto-includes FK deps
pgsync sync <db> --yes          # skip confirmation
pgsync sync <db> --dry-run      # show plan, no writes
pgsync config                   # → TUI config editor (full wizard)
pgsync config show              # read-only print current config (passwords masked)
pgsync config path              # print path to config file (for debug only — не для редактирования)
pgsync config reset             # очистить и пере-настроить через TUI
pgsync doctor                   # diagnostic: tools found, proxy reachable, target writable
pgsync upgrade                  # self-update from GitHub Releases
pgsync version
```

### Глобальные флаги

| Флаг | Default | Описание |
|---|---|---|
| `--output` | `text` | `text` \| `json` (NDJSON для LLM) |
| `--threads` | `runtime.NumCPU()` | параллельных воркеров COPY |
| `--engine` | `auto` | `auto` \| `native` \| `external` |
| `--use-system-pgtools` | `false` | использовать pg_dump из $PATH вместо embedded |
| `--config` | `$HOME/.pgsync.env` | путь к конфиг-файлу |
| `--quiet`, `--verbose` | — | log level |
| `--no-color` | — | для CI/pipe |

### NDJSON (для `--output=json`)

Каждая строка stdout — один JSON-объект:

```json
{"ts":"2026-05-02T10:11:23Z","level":"info","event":"sync.start","db":"ai_pushka_biz","tables":42,"engine":"native"}
{"ts":"...","event":"schema.predata.start"}
{"ts":"...","event":"schema.predata.done","duration_ms":820}
{"ts":"...","event":"table.copy.start","table":"messages","est_rows":1000000}
{"ts":"...","event":"table.copy.progress","table":"messages","rows":120000,"pct":12.0,"bytes_per_sec":42000000}
{"ts":"...","event":"table.copy.done","table":"messages","rows":1000000,"duration_ms":4231}
{"ts":"...","event":"schema.postdata.start"}
{"ts":"...","event":"schema.postdata.done","indexes":17,"fks":12,"duration_ms":2100}
{"ts":"...","event":"sync.done","duration_ms":31420,"tables":42,"bytes":665000000}
{"ts":"...","level":"error","event":"sync.failed","stage":"copy","table":"messages","error":"..."}
```

Stderr в JSON-режиме — пустой или только fatal-ошибки текстом (для `set -e`-friendly).

---

## 7. TUI

Стек: `bubbletea` + `lipgloss` + `huh` (формы).

**Экраны (state-machine):**

```
SettingsCheck → MainMenu → DatabaseList → [TablesPick] → ConfirmPlan → Progress → Result
       ↑           │                                                                    │
       │           └→ ConfigEditor (можно открыть в любой момент клавишей 's')          │
       └─────────────────── back to MainMenu (Esc) ─────────────────────────────────────┘
```

- **SettingsCheck** *(автоматический gate на каждом запуске)* — если конфиг отсутствует или неполный, **сразу** открывается **ConfigEditor** в режиме wizard; пройти его — обязательное условие для входа в основное меню. Если конфиг валидный — экран пропускается.
- **ConfigEditor** — единственный способ менять настройки. Многошаговая huh-форма, разбитая на секции (Remote / Local / Runtime / Logging) с tab-навигацией. Inline-валидация (host reachable, port в диапазоне, password not empty, sslmode из enum). Кнопка «Test connection» проверяет remote и local в реальном времени. «Save» атомарно перезаписывает файл-хранилище. Доступен из MainMenu (`s`) и из любого экрана через шорткат, и через `pgsync config` из CLI.
- **DatabaseList** — multi-select из БД на удалённом, с колонкой size (через `pg_database_size`).
- **TablesPick** *(опционально, если активирован «advanced»)* — multi-select таблиц с авто-разворачиванием по FK.
- **ConfirmPlan** — таблица: db, размер, target action (drop & recreate), engine, threads, время оценочное.
- **Progress** — overall progress + per-table sub-progress (bubbles spinner + bar). Можно поставить на паузу очередь синков, добавить ещё.
- **Result** — суммарная статистика: длительность, MB/s, MB total, rows total, ошибки.

Очередь синков (можно подготовить N БД и запустить пакетом) — переносим из dbsync.

---

## 8. Конфиг

### 8.1 Принцип

Конфиг **полностью управляется TUI** — пользователь никогда руками не открывает файл. Файл — внутренняя деталь хранения, не часть интерфейса.

Пути для редактирования:
- `pgsync` → если конфиг неполный, автоматически открывается ConfigEditor wizard.
- `pgsync config` → ConfigEditor явно (для изменений после первичной настройки).
- В TUI на любом экране: шорткат `s` → ConfigEditor.

### 8.2 Что хранится (логические поля, не env-имена)

| Секция | Поля |
|---|---|
| Remote | host, port, user, password, default database, ssl mode, proxy URL |
| Local | host, port, user, password, ssl mode |
| Runtime | threads, engine (auto/native/external), use_system_pgtools, default_database |
| Logging | level (debug/info/warn/error), format (text/json) |

Каждое поле в TUI имеет: title, описание, default, валидатор. Подсказки и лейблы — на русском, как привычно пользователю.

### 8.3 Хранение

**Default путь:**
- Linux/macOS: `~/.config/pgsync/config.toml`
- Windows: `%APPDATA%\pgsync\config.toml`

(старый dbsync-style `~/.pgsync.env` не используем — TOML лучше для structured-конфига и валидации, плюс XDG-совместимо).

Файл создаётся и атомарно перезаписывается ConfigEditor'ом (write to tmp → fsync → rename). Файловые права 0600 (паролём владеет только текущий пользователь).

Структура файла (для debug, **не для ручного редактирования**):

```toml
[remote]
host = "prod-pg.example.com"
port = 5432
user = "readonly"
password = "..."
database = "ai_pushka_biz"
ssl_mode = "require"
proxy_url = "socks5://proxy.example.com:1080"

[local]
host = "localhost"
port = 5432
user = "postgres"
password = "postgres"
ssl_mode = "disable"

[runtime]
threads = 8
engine = "native"
use_system_pgtools = false
default_database = "ai_pushka_biz"

[logging]
level = "info"
format = "text"
```

### 8.4 Env / CLI override (только для CI и one-shot)

Для CI и автоматизации поддерживаются env-переменные с префиксом `PGSYNC_*` (например `PGSYNC_REMOTE_HOST`) и CLI-флаги. Они **переопределяют сохранённый конфиг на время одного запуска, но не пишут в файл**. Это ad-hoc эскейп-хатч, не основной способ конфигурации.

Приоритет: CLI flag > env var > config file > defaults.

### 8.5 Future: keyring (post-MVP)

Поскольку файл не user-facing, его легко заменить на keyring-storage (Win Credential Manager / macOS Keychain / `secret-service` на Linux) для паролей. Структурный конфиг остаётся в TOML, чувствительные поля — в keyring. Не блокирующее улучшение MVP.

---

## 9. Прокси

Порт `internal/services/proxy_tunnel.go` из dbsync (TCP-туннель через SOCKS5/SOCKS5h/HTTP/HTTPS). Минимальные адаптации:
- Дефолтный порт назначения 5432 вместо 3306.
- Применяется и к проверке подключения (`pgx.Connect`), и к дочернему `pg_dump` (через `host=127.0.0.1 port=<local-tunnel>`).
- Устанавливается перед стартом любого подключения к remote, останавливается на graceful shutdown.

SSL/TLS — через `sslmode` PG-параметра; туннель не вмешивается в TLS-handshake.

---

## 10. Безопасность

1. **Destructive ops только с подтверждением** — `DROP DATABASE` / `DROP SCHEMA CASCADE` требует TUI-confirm или `--yes` в CLI.
2. **Read-only check на source** — перед началом проверяем, что текущая роль не имеет write-grant'ов на любую таблицу источника (`pg_has_role`/`has_table_privilege`); если имеет — warning, не блок.
3. **Пароли никогда в argv** — для дочерних `pg_dump`/`pg_restore` используем `PGPASSWORD` env или passfile, не `-W` в командной строке.
4. **`--dry-run`** — план без выполнения (список БД, список таблиц, размеры, оценка времени).
5. **Проверка target ≠ source** — отказываем, если remote и local конфигурации указывают на один и тот же endpoint (host:port).
6. **TLS**: дефолт для remote — `sslmode=require`, для local — `disable`.

---

## 11. Тестирование и бенчмарки

### 11.1 Coverage policy: **strict 100%**

Минимум **100% line coverage на всех пакетах в `internal/`**. Это формирует архитектуру, не наоборот:

- Все side-effect'ы (запуск дочерних процессов, файловые операции, время, сеть, рандом) — за интерфейсами в `internal/runner/`, `internal/clock/`, etc. В тестах подменяем фейками.
- В CI шаг `coverage-gate` падает если `go tool cover -func=coverage.out | grep -v '100.0%' | grep -v <allow-list>` выдаёт что-то непустое.
- **Allow-list** (единственные исключения, фиксируется в `coverage.allow`):
  - `cmd/pgsync/main.go` — entry point ≤ 10 строк, тестируется через subprocess в integration.
  - `internal/version/version.go` — поля заполняются `-ldflags` при билде.
  - `internal/engine/pgtools/embed_<platform>.go` — каждый файл покрывается на своей OS в CI matrix; cross-OS unreachable-ветки не считаются.
- **Никаких `//go:build coverage_off` или похожих хаков** — если что-то не покрывается, переписываем чтобы покрывалось.

### 11.2 Mocking discipline (для 100% coverage)

В коде заводим интерфейсы для всех внешних зависимостей. Реализации — ручные fake'и в `internal/<pkg>/fake_test.go`, не gomock (Go-идиоматично, проще читать):

```go
// internal/runner/runner.go
type CommandRunner interface {
    Run(ctx context.Context, name string, args []string, env []string) (stdout, stderr []byte, err error)
}
type StreamingRunner interface {
    Stream(ctx context.Context, name string, args []string, env []string) (cmd.Process, error)
}

// internal/clock/clock.go
type Clock interface { Now() time.Time }

// internal/fs/fs.go
type FS interface { ReadFile, WriteFile, Stat, Rename, MkdirAll, ... }

// internal/proxy/dialer.go
type Dialer interface { DialContext(ctx context.Context, network, addr string) (net.Conn, error) }
```

Production-реализации — тонкие враппера над stdlib. Тесты собирают engines с фейками.

### 11.3 Test pyramid

**Unit** (плотность ~70% всех тестов; быстрые, без I/O):
- `pgschema/parser_test.go` — split pg_dump output на pre/post-data, fuzz-тесты на корректность регексп-границ.
- `pgschema/deps_test.go` — топологическая сортировка FK, self-referencing FK, циклы.
- `engine/native/copy_test.go` — прогресс через io.Pipe + fake pgx connections.
- `engine/pgtools/extract_test.go` — извлечение, sha-проверка, повторный запуск (idempotent), corrupt-cache recovery.
- `cli/agent_progress_test.go` — NDJSON формат, schema validator.
- `config/*_test.go` — load/save TOML, atomic write на tmpfs, env-override приоритет, валидаторы для huh.
- `proxy/tunnel_test.go` — fake dialer, порядок старт/стоп.
- `tui/screens/*_test.go` — bubbletea TestModel'ы для экранов (huh formfield validation, navigation, edge cases).
- Общие fuzz-таргеты: парсер pg_dump output, конфиг-валидаторы, NDJSON encoder.

**Integration** (~20%; testcontainers-go поднимает 2 чистых `postgres:18` контейнера):
- **Happy path** на каждом фикстуре (tiny / small / medium): наливаем → sync → row counts совпадают, md5 каждой таблицы совпадает, sequences совпадают, индексы созданы.
- **Partial sync:** `--tables messages,users` — авто-FK закрытие, корректный порядок.
- **Proxy:** третий контейнер с `serjs/go-socks5-proxy`; прогон через него + negative-test без прокси.
- **Interrupt:** SIGINT посреди copy → корректный rollback на target, фикс ошибки, retry проходит.
- **Schema-only / Data-only режимы**, **ANALYZE**, **`--concurrent-indexes`**.
- **Сравнение со cистемным pg_dump+pg_restore** на одном фикстуре — наш COPY BINARY pipeline должен быть быстрее.

**E2E** (~5%; manual + опционально CI):
- Реальный прогон против прод `ai_pushka_biz` через прокси — пишем результат в `benchmarks/results/<sha>/ai-real.json`.
- Smoke на каждом релизе перед `pgsync upgrade`.

**Benchmarks** (~5%; см. 11.5).

### 11.4 Тестовые фикстуры

Гибрид «свой генератор + 1 публичный baseline». Все фикстуры воспроизводимы из кода.

| Фикстура | Размер | Что внутри | Источник |
|---|---|---|---|
| **tiny** | ~50 KB | 3 таблицы, FK chain, ~500 rows, 1 sequence | свой `pgsync-genfixture --size=tiny` |
| **small** | ~15 MB | dvdrental (классика, 15 таблиц, sequences, views) | публичный — postgresqltutorial |
| **medium** | ~500 MB | 50 таблиц, jsonb, arrays, ENUMs, partial indexes, GIN, ~2M rows | свой gen |
| **large** | ~5 GB | 100 таблиц, partitioned, materialized views, GIN на jsonb, FK-каскады, ~50M rows | свой gen |

xl > 5 GB сознательно вне scope (storage cost на проде, время прогона в CI).

#### `pgsync-genfixture` (отдельная утилита в репо)

```bash
pgsync-genfixture --size=large --seed=42 --out=fixtures/large.sql
```

- Детерминированный (один seed → один и тот же дамп везде).
- На выходе: `.sql.gz` (для git LFS — нет; будем хранить как генератор + сидованный prng в коде, дампы — гитигнор).
- `fixtures/upload-prod.sh` — один-shot заливает `tiny/small/medium/large` в твой прод-кластер под имена `pgsync_fixture_<size>`.

CI поднимает фикстуры через `docker exec psql -f` в контейнер, не качает с прода.

### 11.5 Benchmark suite

Отдельный Go-пакет `benchmarks/`. Запуск: `go test -bench=. -run=^$ ./benchmarks/...` или `make bench`.

**Метрики на каждый прогон:**
- Wall-clock duration (total).
- Per-stage breakdown: snapshot / pre-data / copy / post-data / sequences / analyze.
- Throughput: rows/s, MB/s.
- Per-table top-10 slowest.
- Memory peak (`runtime.MemStats`).
- CPU saturation per worker (опц).

**Где живут результаты:**

```
benchmarks/
├── results/
│   ├── <git-sha>/
│   │   ├── tiny.json
│   │   ├── small.json
│   │   ├── medium.json
│   │   ├── large.json
│   │   └── ai-real.json   (e2e на прод-БД)
│   └── HISTORY.md         (автогенерируемый markdown с trendline)
├── runner_test.go         (testing.B harness)
└── compare.go             (CLI для diff-а двух прогонов)
```

Формат `<fixture>.json`:

```json
{
  "schema_version": 1,
  "fixture": "medium",
  "git_sha": "abc1234",
  "git_dirty": false,
  "host": {"os":"darwin","arch":"arm64","cpu":"M2","cores":8,"ram_gb":16},
  "engine": "native",
  "threads": 8,
  "duration_ms": 31420,
  "stages_ms": {"snapshot":50,"predata":820,"copy":26100,"postdata":4200,"sequences":250},
  "throughput": {"rows_per_sec":32000,"bytes_per_sec":158000000},
  "rows_total": 2000000,
  "bytes_total": 524288000,
  "tables": {"total":50,"slowest":[{"name":"events","ms":4200}]},
  "memory": {"peak_mb":420}
}
```

**Регрессия в CI:**
- Шаг `bench-regression` запускает tiny + small + medium на свежем checkout, сравнивает с baseline в `benchmarks/results/main/`.
- Падает если **duration > baseline + 15%** на любом фикстуре, или throughput < baseline − 15%.
- Comparison через `benchstat` (стандартный Go-инструмент) для статистической значимости + наш `compare.go` для агрегата.
- На каждом merge в main — обновляем baseline в `main/`.
- `large` (5 GB) не гоняем в CI каждый раз — только при тегировании релиза или вручную через `workflow_dispatch`.

**Цель производительности (revised):**
- `medium` (500 MB / 2M rows): **≤ 30 s** на M2 8c (для сравнения: dbsync 32 s на 665 MB MySQL).
- `large` (5 GB / 50M rows): **≤ 4 min** на M2 8c.

---

## 12. Modern Go conventions (1.25+)

Обязательно используем где идиоматично:

- **`log/slog`** — единственный логгер; никаких `log.Printf`, никаких глобалов.
- **`iter.Seq` / range-over-func** (1.23+) — итерация по таблицам, прогрессу, парсингу dump output.
- **`slices` / `maps`** stdlib — заменяют ручные циклы (`slices.SortFunc`, `slices.Contains`, `maps.Keys`).
- **`errors.Join`** — агрегация ошибок параллельных воркеров.
- **`context.AfterFunc`** — graceful cleanup туннелей и пулов.
- **`cmp.Or`** — дефолты в конфиге.
- **Generics** — `pkg/utils/`: `Set[T]`, `MapKeys[K,V]`, generic worker pool.
- **`go test -fuzz`** — обязательно для парсера pg_dump output, валидаторов, NDJSON encoder/decoder.
- **`testing.T.Context()`** (1.24+) — авто-cancel в tests.

Запрещено: `interface{}` где можно `any`, `ioutil` (deprecated), ручной `sort.Slice`/`sort.SliceStable` где есть `slices.Sort*`, глобальный `rand.Seed`/`rand.Int*` (только инжектируемый `*rand.Rand`).

Линтеры (CI gate): `golangci-lint` со включёнными `revive`, `gocritic`, `staticcheck`, `gosec`, `gocognit` (max=15), `gocyclo` (max=10), `errorlint`, `nilerr`, `prealloc`, `wastedassign`, `unparam`. Конфиг — `.golangci.yml` в корне.

---

## 13. Распространение

GitHub Releases (как dbsync):
- `pgsync-windows-amd64.zip`
- `pgsync-darwin-arm64.tar.gz`
- `pgsync-darwin-amd64.tar.gz`
- `pgsync-linux-amd64.tar.gz`

Self-update: команда `pgsync upgrade` — порт `internal/updater/` из dbsync (GitHub Releases API → скачать → swap binary → restart).

CI (GitHub Actions):
1. `lint` — `golangci-lint` (строгий конфиг, см. §12).
2. `test-unit` — на всех платформах в matrix (windows/macos/linux × amd64/arm64 где есть).
3. `test-integration` — Linux only (testcontainers).
4. `coverage-gate` — падает если coverage < 100% (минус allow-list, см. §11.1).
5. `bench-regression` — tiny+small+medium, fail если регрессия > 15%.
6. `build-all` — на тег `v*`, заливает в Releases.
7. `release-notes` — генерируются из CHANGELOG.

---

## 14. Закрытые решения

- **Min PG: 18** (source и target). PG <18 не таргетим (см. §2).
- **Embedded pg_tools: 18** (matching minimum target — без рисков несовместимости).
- **`CREATE INDEX CONCURRENTLY`** — дефолт **off**, флаг `--concurrent-indexes` включает.
- **Coverage: strict 100%** на `internal/`, allow-list зафиксирован (§11.1).
- **Fixture max: large = 5 GB.** xl > 5 GB вне scope.
- **Keyring** для паролей — post-MVP (§8.5).

---

## 15. Связь с dbsync (что переносим)

| dbsync файл | pgsync эквивалент | Что меняем |
|---|---|---|
| `internal/services/proxy_tunnel.go` | `internal/proxy/tunnel.go` | дефолтный порт 5432, минор |
| `internal/services/database.go` (ListDatabases, ListTables, FK-deps) | `internal/pgschema/*.go` | переписать SQL под `pg_catalog` |
| `internal/services/mysqlshell.go` | `internal/engine/native/*.go` | новая логика (pgx + COPY вместо mysqlsh) |
| `internal/cli/commands.go`, `agent_progress.go`, `plain_output.go` | `internal/cli/*` | прямой порт |
| `internal/tui/app.go` | `internal/tui/app.go` + screens | + huh формы поверх |
| `internal/config/*` | `internal/config/*` | TOML-хранилище, TUI-only управление, env-override опционально |
| `internal/updater/*` | `internal/updater/*` | прямой порт |
| `internal/version/*` | `internal/version/*` | прямой порт |
| `internal/models/*` | `internal/models/*` | адаптация |

Параллель сохраняем сознательно: фиксы, найденные в одном проекте, легко применять во второй.

---

## 16. Tech stack (свежие, активно поддерживаемые)

| Слой | Lib | Версия | Почему |
|---|---|---|---|
| Go | `go` | 1.25+ | latest stable, как dbsync |
| CLI | `spf13/cobra` | v1.9+ | стандарт, в dbsync уже работает |
| Config | `BurntSushi/toml` | v1.4+ | минималистичный, надёжный TOML; viper не нужен (нет YAML/JSON-конфигов, нет watch) |
| TUI core | `charmbracelet/bubbletea` | v1.3+ | лучший Go-TUI фреймворк |
| TUI forms | `charmbracelet/huh` | v0.6+ | декларативные формы — заменяет ручные модели для ConfigEditor / select-DB / multi-select tables |
| TUI styles | `charmbracelet/lipgloss` | v1.1+ | styling, в dbsync уже |
| PG driver | `jackc/pgx` | v5.7+ | де-факто стандарт, нативный COPY BINARY, snapshot ID, parallel-safe |
| Logger | `log/slog` (stdlib) + `charmbracelet/log` | latest | structured (JSON для агента) + красивый human-out |
| Proxy | `golang.org/x/net/proxy` | latest | SOCKS5/SOCKS5h dialer, в dbsync уже |
| Tests | `stretchr/testify` + `testcontainers-go` | v1.10 / v0.36+ | reproducible PG в integration |
| Bench | stdlib `testing.B` + `golang.org/x/perf/cmd/benchstat` | latest | статистическая значимость регрессий |
| Coverage | stdlib `cover` + custom `coverage-gate.sh` | — | 100% gate с allow-list |
| Linter | `golangci-lint` | v2+ | строгий профиль (§12) |
| Build | stdlib `embed`, build tags | — | per-platform pg-tools embedding |

**Сознательно не берём:**
- `viper` — overkill для одного TOML-файла, добавляет 30+ транзитивных deps
- `urfave/cli` — cobra+API уже знаком из dbsync
- `ratatui-go`/прочие TUI — bubbletea выигрывает по экосистеме
- `lib/pq` — устаревший по сравнению с pgx
- `goreleaser` — пока хватит ручного Makefile/build.ps1, как в dbsync

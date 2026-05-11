# pgsync — Infisical Per-Project DB-name Resolver

**Дата:** 2026-05-11
**Автор:** Kiril (taborrnd@gmail.com), assisted by Claude
**Статус:** Approved (brainstorming → spec)
**Связано с:** `2026-05-02-pgsync-design.md` (исходный дизайн)

---

## 1. Контекст и проблема

Все 5 активных проектов (`ai`, `kp`, `easy2`, `masterm`, `octane`) переехали с `.env` на Infisical. Текущий резолвер pgsync читает `.env` из cwd и парсит `POSTGRES_URL` / `DB_DATABASE` оттуда (`internal/cli/config_resolver.go:227`, `internal/config/override.go:58`). После миграции эти файлы либо отсутствуют, либо устарели — `pgsync sync` без аргументов перестал работать.

Цель: восстановить однокомандный флоу `pgsync sync` из любого проектного каталога, читая имя БД из Infisical.

---

## 2. Модель данных

### 2.1 Что неизменно (живёт в pgsync config.toml)

Кластерные креды — общие для всех проектов и редко меняются:

- **`[remote]`** — один shared DigitalOcean Managed PG кластер
  - `host = "private-db-postgresql-1-do-user-3397877-0.h.db.ondigitalocean.com"`
  - `port = 25060`
  - `user = "doadmin"`
  - `password = "<AVNS_…>"`
  - `sslmode = "require"`
- **`[local]`** — локальный Postgres
  - `host = "localhost"`
  - `port = 5432`
  - `user = "mttzzzz"`
  - (без пароля)

Файл хранится по стандартному пути pgsync (`~/.config/pgsync/config.toml` на Linux/Mac, `%APPDATA%\pgsync\config.toml` на Windows). Пишется один раз руками либо через существующие `pgsync config` команды. Поля `[remote].database` и `[local].database` **намеренно не задаются** — резолвятся per-call.

### 2.2 Что меняется per-project (живёт в Infisical)

Per-project нам нужно ровно одно поле — **имя БД**. Имена БД совпадают между prod и local (`ai_pushka_biz`, `masterm_pushka_biz`, …), поэтому хватает одного источника.

Источник — Infisical env `dev`, та же `workspaceId` что и для приложения проекта. Переменные:

- **Nuxt-проекты** (`ai`, `kp`, `easy2`) — поле `POSTGRES_URL`, имя БД = path-компонент URL.
- **Laravel-проекты** (`masterm`, `octane`) — поле `DB_DATABASE`, имя БД = значение напрямую.

Prod-env Infisical может содержать дополнительные поля (`DB_HOST`, `DB_PG_HOST`, …) — pgsync их **не использует**.

---

## 3. Архитектура резолвера

### 3.1 Точка входа

`pgsync sync` без аргументов запускает Infisical-резолвер. Алгоритм:

1. **Найти `.infisical.json`**, идя от текущего cwd вверх по дереву каталогов. Найденный каталог = «корень проекта».
2. **Проверить наличие `infisical` бинаря в `PATH`.** Если нет — fail fast с сообщением `pgsync: 'infisical' CLI not found in PATH (install: https://infisical.com/docs/cli/overview)`.
3. **Из корня проекта** выполнить:
   ```
   infisical export --env=dev --format=dotenv --silent
   ```
   Парсить stdout как стандартный dotenv (тот же парсер, что сейчас в `loadDotEnv`).
4. **Извлечь имя БД** в порядке приоритета:
   - Если `POSTGRES_URL` непустой → разобрать как URL, взять path-компонент без ведущего `/`.
   - Иначе если `DB_DATABASE` непустой → использовать как есть.
   - Иначе fail fast: `pgsync: cannot resolve DB name from Infisical (env=dev): neither POSTGRES_URL nor DB_DATABASE is set`.
5. **Подставить имя в `cfg.Remote.Database`, `cfg.Local.Database` и `cfg.Runtime.DefaultDatabase`.**
6. Дальше — существующий sync-флоу без изменений.

### 3.2 Override / bypass

Резолвер запускается только если имя БД ещё не задано после слоёв defaults → TOML → env → flags (см. §4.2). То есть автоматически bypass'ится во всех трёх явных случаях:

- `pgsync sync <dbname>` — positional argument попадает в `cfg.Runtime.DefaultDatabase` через существующий путь.
- `[remote] database = "x"` в `config.toml` — задан до резолвера.
- `PGSYNC_REMOTE_DATABASE=x` или `--remote-database=x` — то же.

Это даёт чистый escape hatch для non-Infisical проектов / отладки, без отдельных флагов.

### 3.3 Поведение при ошибках (fail-fast)

Все четыре кейса валятся с конкретными сообщениями, без fallback'ов:

| Ситуация | Сообщение |
|---|---|
| `.infisical.json` не найден, идя вверх от cwd | `pgsync: no .infisical.json found walking up from <cwd>` |
| `infisical` не в PATH | `pgsync: 'infisical' CLI not found in PATH` |
| `infisical export` вернул ненулевой код | `pgsync: infisical export failed (exit N): <stderr>` |
| Ни `POSTGRES_URL`, ни `DB_DATABASE` в выводе | `pgsync: cannot resolve DB name from Infisical (env=dev)` |

Принцип: если что-то пошло не так — пользователь должен понять что именно и где, без молчаливых fallback'ов на пустые/непредсказуемые значения.

---

## 4. Компоненты и границы

### 4.1 Новый пакет `internal/secrets/infisical`

Изолированный модуль с одним публичным методом:

```go
package infisical

type Resolver struct {
    CWD      string
    LookPath func(string) (string, error)  // exec.LookPath, инъекция для тестов
    Run      func(ctx context.Context, dir string, name string, args ...string) ([]byte, error)
}

// ResolveDBName walks up from r.CWD looking for .infisical.json, runs
// `infisical export --env=dev --format=dotenv --silent`, and extracts
// the database name from POSTGRES_URL or DB_DATABASE.
func (r Resolver) ResolveDBName(ctx context.Context) (string, error)
```

`LookPath` и `Run` — инжектируемые для unit-тестов (стабим `exec.Command` так, как уже сделано в других местах pgsync — см. `internal/engine/system_tools_test.go`).

### 4.2 Интеграция в `internal/cli/config_resolver.go`

В `Resolve()` после `applyFlagOverrides` добавляется шаг:

```go
if !databasesProvided(cfg, flags) {
    name, err := infisical.NewResolver().ResolveDBName(ctx)
    if err != nil {
        return config.Config{}, err
    }
    cfg.Remote.Database = name
    cfg.Local.Database = name
    cfg.Runtime.DefaultDatabase = name
}
```

`databasesProvided` смотрит, заданы ли `cfg.Remote.Database` и `cfg.Local.Database` (из конфига или флагов). Если задано — резолвер пропускается. Это сохраняет совместимость с `pgsync sync <dbname>` и с явным `[remote] database = "…"` в TOML.

### 4.3 Удаляется

- `loadDotEnv`, `parseDotEnvLine`, `parseDotEnvValue` из `config_resolver.go` — нам больше не нужен `.env`-loader.
- `applyConventionalEnv` из `internal/config/override.go` — `DB_DATABASE` / `POSTGRES_URL` из process env больше не магически подставляются в local. (Если кто-то реально захочет это поведение — `PGSYNC_LOCAL_*` остаются, как и флаги.)
- Соответствующие тесты на dotenv-loading.

`PGSYNC_*` namespaced env vars остаются — это явный escape hatch для CI / редких случаев.

---

## 5. Конфигурация — пример сценария

**Раз-в-жизни setup пользователя:**

```bash
# Заполнить ~/.config/pgsync/config.toml единожды (через `pgsync config edit`
# или руками). Содержит cluster-creds prod + local, без database.
cat > ~/.config/pgsync/config.toml <<EOF
[remote]
host     = "private-db-postgresql-1-do-user-3397877-0.h.db.ondigitalocean.com"
port     = 25060
user     = "doadmin"
password = "AVNS_…"
sslmode  = "require"

[local]
host = "localhost"
port = 5432
user = "mttzzzz"
EOF

# Залогиниться в Infisical (если ещё не залогинен на этой машине)
infisical login
```

**Per-project use:**

```bash
cd ~/projects/ai.pushka.biz
pgsync sync                      # резолвит ai_pushka_biz из Infisical:dev → sync
pgsync sync some_other_db        # явный override, Infisical не дергается

cd ~/projects/masterm.pushka.biz
pgsync sync                      # резолвит masterm_pushka_biz через DB_DATABASE
```

---

## 6. Тестирование

Три уровня по правилу проекта.

### 6.1 Unit (`internal/secrets/infisical/resolver_test.go`)

- Walk-up к `.infisical.json` — корректно находит ближайший родительский каталог; нет файла → ошибка с понятным сообщением.
- Парсинг dotenv-вывода: `POSTGRES_URL=postgresql://u:p@h:5432/dbname?sslmode=require` → `dbname`; quoted значения; пустые строки; комментарии.
- Приоритет: `POSTGRES_URL` побеждает `DB_DATABASE`, если оба заданы.
- Mock `Run` возвращает stdout/stderr/exitcode — проверяем что fail-fast сообщения соответствуют контракту таблицы в §3.3.
- `LookPath` возвращает `exec.ErrNotFound` → корректная ошибка про PATH.

### 6.2 Integration (`internal/cli/config_resolver_test.go`)

- `Resolver.Resolve(ctx, flags)` в temp dir с фейковым `.infisical.json` и stubbed `Run` — успешно подставляет database в `cfg.Remote` и `cfg.Local`.
- `pgsync sync <name>` — override-путь, Run **не должен** быть вызван.
- TOML с явным `[remote] database = "x"` — Run **не должен** быть вызван.

### 6.3 e2e (`cmd/pgsync/sync_e2e_test.go`)

- Реальный бинарь pgsync, `--dry-run`, реальный walk-up к `.infisical.json` во временном дереве.
- Чтобы избежать зависимости от прод-Infisical в CI: создаём в temp dir fake `infisical` shell-script, кладём его в `PATH` перед прогоном (через `t.Setenv("PATH", ...)`). Скрипт пишет фиксированный dotenv-вывод на stdout. Это проверяет всю цепочку pgsync→exec→parser без сетевых вызовов.
- Проверяем что в stdout pgsync звучит ожидаемое имя БД в plan-сообщении (`Sync: db=<name> remote=<host> → local=<host>`) и что exit code 0.

### 6.4 Что **не** тестируем

- Реальный `infisical export` против прод-workspace — это flakey по причинам сети и небезопасно для тестов в CI.
- Поведение в проектах с экзотическими структурами `POSTGRES_URL` (multi-host, libpq-style key/value) — out of scope; падаем явной ошибкой.

---

## 7. Не-цели этого спринта

- Поддержка других секрет-провайдеров (Vault, 1Password, AWS SM) — добавим интерфейс позже, если понадобится. Сейчас `infisical` хардкод.
- Авто-генерация / редактирование `config.toml` при первом запуске — пользователь заполняет сам. (Можно отдельной задачей добавить `pgsync config bootstrap` если будет нужно.)
- Поддержка кастомного Infisical env (не `dev`) — захардкожен `dev`, обсуждаемо при появлении staging.
- Кэширование результата `infisical export` — sync редкая команда, лишняя сложность.
- Параллельный pull нескольких БД — отдельная фича.

---

## 8. Риски и mitigations

| Риск | Mitigation |
|---|---|
| Infisical token истёк → `infisical export` падает | Fail-fast с явным сообщением `infisical export failed (exit N): <stderr>` — пользователь видит `Unauthorized` и понимает что нужно `infisical login` |
| Пользователь забыл `infisical login` на новой машине | То же — fail-fast, понятный stderr |
| Workspace `.infisical.json` указывает на другой workspace, чем нужно | Out of scope — это user error; pgsync доверяет файлу |
| Имена БД prod ≠ local (например, разработчик переименовал локалку) | Использовать override `pgsync sync <local-name>` или прописать `[local] database = "…"` в `config.toml` (но это глобально — лучше переименовать локалку обратно) |
| `infisical` CLI поменяет формат вывода `--format=dotenv` | Unit-тесты на парсер фиксируют ожидаемый формат; при breaking change pgsync падает на парсинге, не молча |

---

## 9. Скилл-интеграция (вне scope этого PR, но мотивация)

После этого изменения проектные скилы и `pgsync sync` в `CLAUDE.md` записях упрощаются до:

```
cd <project-root> && pgsync sync
```

— одна команда, без `infisical run -- …`, без проброса env. Это и есть точка боли, которую закрывает спека.

package screens

import (
	"context"
	"fmt"
	"strconv"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"

	"github.com/mttzzz/pgsync/internal/config"
)

// ConfigEditorMode selects editor behavior.
type ConfigEditorMode string

const (
	// WizardMode is required first-run setup.
	WizardMode ConfigEditorMode = "wizard"
	// EditMode edits an existing config.
	EditMode ConfigEditorMode = "edit"
	// ResetMode starts from defaults after reset.
	ResetMode ConfigEditorMode = "reset"
)

// ConfigEditorStore persists edited config.
type ConfigEditorStore interface {
	Save(ctx context.Context, cfg config.Config) error
}

// ConfigEditor edits a draft config.
type ConfigEditor struct {
	Mode   ConfigEditorMode
	Draft  config.Config
	Saved  bool
	Status string
	Err    error
	Store  ConfigEditorStore
}

// NewConfigEditor creates an editor with mode-specific defaults.
func NewConfigEditor(cfg config.Config, mode ConfigEditorMode, store ConfigEditorStore) ConfigEditor {
	if mode == ResetMode {
		cfg = config.Defaults()
	}
	return ConfigEditor{Mode: mode, Draft: cfg, Store: store}
}

// Init returns no command.
func (e ConfigEditor) Init() tea.Cmd { return nil }

// Update handles save/cancel shortcuts.
func (e ConfigEditor) Update(msg tea.Msg) (Screen, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return e, nil
	}
	switch key.String() {
	case "ctrl+s":
		return e.Save(context.Background()), nil
	case "esc":
		if e.Mode != WizardMode {
			e.Status = "Изменения отменены"
		}
	}
	return e, nil
}

// View renders a redacted summary.
func (e ConfigEditor) View() string {
	redacted := config.Redacted(e.Draft)
	return fmt.Sprintf("Remote: %s\nLocal: %s\nStatus: %s", redacted.Remote.Host, redacted.Local.Host, e.Status)
}

// ID returns ConfigEditorID.
func (e ConfigEditor) ID() ID { return ConfigEditorID }

// Title returns title.
func (e ConfigEditor) Title() string { return "Настройки" }

// Help returns key help.
func (e ConfigEditor) Help() string { return "ctrl+s сохранить · esc назад" }

// Set updates a draft field.
func (e ConfigEditor) Set(key, value string) ConfigEditor {
	switch key {
	case "remote.host":
		e.Draft.Remote.Host = value
	case "local.host":
		e.Draft.Local.Host = value
	case "remote.user":
		e.Draft.Remote.User = value
	case "local.user":
		e.Draft.Local.User = value
	}
	return e
}

// Save validates and persists the draft.
func (e ConfigEditor) Save(ctx context.Context) ConfigEditor {
	if err := config.Validate(e.Draft); err != nil {
		e.Err = err
		e.Status = "Исправьте ошибки"
		return e
	}
	if e.Store != nil {
		if err := e.Store.Save(ctx, e.Draft); err != nil {
			e.Err = err
			e.Status = "Не удалось сохранить"
			return e
		}
	}
	e.Saved = true
	e.Err = nil
	e.Status = "Настройки сохранены"
	return e
}

// BuildConfigForm constructs a huh form for production wiring.
func BuildConfigForm(cfg config.Config, mode ConfigEditorMode) *huh.Form {
	return BuildEditableConfigForm(&cfg, mode)
}

// BuildEditableConfigForm constructs an interactive form bound to cfg.
func BuildEditableConfigForm(cfg *config.Config, mode ConfigEditorMode) *huh.Form {
	if cfg == nil {
		defaults := config.Defaults()
		cfg = &defaults
	}
	if mode == ResetMode {
		*cfg = config.Defaults()
	}

	return huh.NewForm(
		huh.NewGroup(
			huh.NewNote().
				Title(configFormTitle(mode)).
				Description("Заполните подключения к удалённому и локальному PostgreSQL. Пароли сохраняются только в локальный TOML-конфиг с правами 0600.").
				Next(true).
				NextLabel("Начать"),
		),
		huh.NewGroup(
			huh.NewInput().Title("Хост prod").Description("Адрес удалённого PostgreSQL").Value(&cfg.Remote.Host).Validate(config.ValidateHost),
			huh.NewInput().Title("Порт prod").Description("Порт удалённого PostgreSQL").Accessor(intStringAccessor{value: &cfg.Remote.Port}).Validate(validatePortString),
			huh.NewInput().Title("Пользователь prod").Description("Пользователь для чтения").Value(&cfg.Remote.User).Validate(requiredString),
			huh.NewInput().Title("Пароль prod").Description("Пароль удалённой БД").Value(&cfg.Remote.Password).EchoMode(huh.EchoModePassword),
			huh.NewInput().Title("База по умолчанию").Description("Необязательно: база для синхронизации по умолчанию").Value(&cfg.Remote.Database),
			huh.NewSelect[string]().Title("SSL mode prod").Description("libpq sslmode").Options(huh.NewOptions("disable", "require", "verify-ca", "verify-full")...).Value(&cfg.Remote.SSLMode).Validate(config.ValidateSSLMode),
			huh.NewInput().Title("Прокси").Description("Необязательно: SOCKS5/HTTP proxy URL").Value(&cfg.Remote.ProxyURL).Validate(config.ValidateProxyURL),
		),
		huh.NewGroup(
			huh.NewInput().Title("Хост local").Description("Адрес локального PostgreSQL").Placeholder("localhost").Value(&cfg.Local.Host).Validate(config.ValidateHost),
			huh.NewInput().Title("Порт local").Description("Порт локального PostgreSQL").Accessor(intStringAccessor{value: &cfg.Local.Port}).Validate(validatePortString),
			huh.NewInput().Title("Пользователь local").Description("Пользователь локальной БД").Value(&cfg.Local.User).Validate(requiredString),
			huh.NewInput().Title("Пароль local").Description("Пароль локальной БД").Value(&cfg.Local.Password).EchoMode(huh.EchoModePassword),
			huh.NewSelect[string]().Title("SSL mode local").Description("libpq sslmode").Options(huh.NewOptions("disable", "require", "verify-ca", "verify-full")...).Value(&cfg.Local.SSLMode).Validate(config.ValidateSSLMode),
		),
		huh.NewGroup(
			huh.NewInput().Title("Потоки").Description("Количество потоков COPY").Accessor(intStringAccessor{value: &cfg.Runtime.Threads}).Validate(validatePositiveInt),
			huh.NewSelect[string]().Title("Движок").Description("native/external/auto").Options(huh.NewOptions("native", "external", "auto")...).Value(&cfg.Runtime.Engine).Validate(validateEngineString),
			huh.NewInput().Title("База по умолчанию").Description("Необязательно: если команда без db").Value(&cfg.Runtime.DefaultDatabase),
			huh.NewConfirm().Title("Использовать системные pg_dump/pg_restore из PATH?").Value(&cfg.Runtime.UseSystemPgtools).Affirmative("Да").Negative("Нет"),
			huh.NewConfirm().Title("Создавать индексы конкурентно, когда возможно?").Value(&cfg.Runtime.ConcurrentIndexes).Affirmative("Да").Negative("Нет"),
		),
		huh.NewGroup(
			huh.NewSelect[string]().Title("Уровень логов").Options(huh.NewOptions("debug", "info", "warn", "error")...).Value(&cfg.Logging.Level).Validate(validateLogLevel),
			huh.NewSelect[string]().Title("Формат логов").Options(huh.NewOptions("text", "json")...).Value(&cfg.Logging.Format).Validate(validateLogFormat),
			huh.NewNote().Title("Готово").Description("Нажмите Enter, чтобы сохранить настройки.").Next(true).NextLabel("Сохранить"),
		),
	)
}

func configFormTitle(mode ConfigEditorMode) string {
	switch mode {
	case WizardMode:
		return "Первичная настройка pgsync"
	case ResetMode:
		return "Сброс и новая настройка pgsync"
	default:
		return "Настройки pgsync"
	}
}

type intStringAccessor struct {
	value *int
}

func (a intStringAccessor) Get() string {
	if a.value == nil {
		return ""
	}
	return strconv.Itoa(*a.value)
}

func (a intStringAccessor) Set(value string) {
	if a.value == nil {
		return
	}
	parsed, err := strconv.Atoi(value)
	if err == nil {
		*a.value = parsed
	}
}

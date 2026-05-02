package screens

import (
	"context"
	"fmt"

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
	return huh.NewForm(huh.NewGroup(huh.NewNote().Title("Настройки").Description(string(mode) + ": " + cfg.Remote.Host)))
}

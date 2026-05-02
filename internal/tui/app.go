package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/mttzzz/pgsync/internal/config"
	"github.com/mttzzz/pgsync/internal/tui/screens"
)

// State is the TUI app state.
type State struct {
	Current screens.ID
	Config  config.Config
	Status  string
	Err     error
	Quit    bool
	Running bool
}

// App is a pure Bubble Tea model shell.
type App struct {
	state State
}

// NewApp creates the default TUI app model.
func NewApp(cfg config.Config) App {
	app := App{state: State{Current: screens.SettingsCheckID, Config: cfg}}
	if err := config.Validate(cfg); err != nil {
		app.state.Current = screens.ConfigEditorID
		app.state.Err = err
		app.state.Status = "Нужно настроить подключение перед первым запуском"
		return app
	}
	app.state.Current = screens.MainMenuID
	app.state.Status = "Настройки загружены"
	return app
}

// Init returns no startup command; callers can inject settings messages in tests.
func (a App) Init() tea.Cmd { return nil }

// Update handles global navigation messages.
func (a App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {
	case SettingsLoadedMsg:
		return a.onSettings(m), nil
	case SyncFinishedMsg:
		a.state.Running = false
		a.state.Current = screens.ResultID
		a.state.Err = m.Err
		if m.Result != nil {
			a.state.Status = fmt.Sprintf("Готово: %s", m.Result.Duration())
		}
		return a, nil
	case tea.KeyMsg:
		return a.onKey(m)
	default:
		return a, nil
	}
}

// View renders the current app shell.
func (a App) View() string {
	body := a.screenBody()
	if a.state.Status != "" {
		body += "\n\n" + a.state.Status
	}
	if a.state.Err != nil {
		body += "\nОшибка: " + a.state.Err.Error()
	}
	return body
}

func (a App) screenBody() string {
	switch a.state.Current {
	case screens.ConfigEditorID:
		return screens.NewConfigEditor(a.state.Config, screens.WizardMode, nil).View() + "\n\nПодсказка: заполните конфиг через `pgsync config` или TOML-файл."
	case screens.MainMenuID:
		return screens.MainMenu().View()
	case screens.DatabaseListID:
		return screens.DatabaseList(nil, nil).View()
	case screens.TablesPickID:
		return screens.TablesPick(nil).View()
	case screens.ConfirmPlanID:
		return screens.ConfirmPlan(nil).View()
	case screens.ProgressID:
		return screens.Progress("ожидание", 0).View()
	case screens.ResultID:
		return screens.Result(nil).View()
	default:
		return fmt.Sprintf("Экран: %s", a.state.Current)
	}
}

// State returns a copy of current state for tests and screen adapters.
func (a App) State() State { return a.state }

func (a App) onSettings(msg SettingsLoadedMsg) App {
	if msg.Err != nil {
		a.state.Current = screens.ConfigEditorID
		a.state.Err = msg.Err
		a.state.Status = "Нужно настроить подключение перед первым запуском"
		return a
	}
	if err := config.Validate(msg.Config); err != nil {
		a.state.Current = screens.ConfigEditorID
		a.state.Err = err
		a.state.Status = "Конфиг неполный, проверьте поля"
		return a
	}
	a.state.Config = msg.Config
	a.state.Current = screens.MainMenuID
	a.state.Err = nil
	a.state.Status = "Настройки загружены"
	return a
}

func (a App) onKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch GlobalKeyAction(msg) {
	case KeyOpenConfig:
		a.state.Current = screens.ConfigEditorID
		a.state.Status = "Настройки"
	case KeyBack:
		if a.state.Current != screens.ConfigEditorID || a.state.Err == nil {
			a.state.Current = screens.MainMenuID
		}
	case KeyQuit:
		if a.state.Running {
			a.state.Status = "Синхронизация выполняется; подтвердите отмену"
		} else {
			a.state.Quit = true
			return a, tea.Quit
		}
	case KeyConfirm:
		a.state.Current = nextScreen(a.state.Current)
	case KeyTogglePause:
		a.state.Status = "Пауза/продолжить"
	}
	return a, nil
}

func nextScreen(id screens.ID) screens.ID {
	switch id {
	case screens.SettingsCheckID:
		return screens.MainMenuID
	case screens.MainMenuID:
		return screens.DatabaseListID
	case screens.DatabaseListID:
		return screens.ConfirmPlanID
	case screens.TablesPickID:
		return screens.ConfirmPlanID
	case screens.ConfirmPlanID:
		return screens.ProgressID
	case screens.ProgressID:
		return screens.ResultID
	default:
		return id
	}
}

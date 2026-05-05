package tui

import (
	"context"
	"fmt"
	"sort"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	zone "github.com/lrstanley/bubblezone"

	"github.com/mttzzz/pgsync/internal/config"
	"github.com/mttzzz/pgsync/internal/engine"
	"github.com/mttzzz/pgsync/internal/models"
	"github.com/mttzzz/pgsync/internal/tui/screens"
)

// State is the TUI app state.
type State struct {
	Current           screens.ID
	Config            config.Config
	Status            string
	Err               error
	Quit              bool
	Running           bool
	MenuIndex         int
	Databases         []models.Database
	DatabaseIndex     int
	SelectedDatabases map[string]bool
	Result            *models.SyncResult
	Results           []*models.SyncResult
	ProgressEvent     engine.Event
	Progress          LiveProgress
	ProgressEvents    <-chan engine.Event
	SyncDone          <-chan SyncFinishedMsg
	ActiveTab         int
	Width             int
	Height            int
}

// App is a pure Bubble Tea model shell.
type App struct {
	state    State
	services Services
}

// NewApp creates the default TUI app model.
func NewApp(cfg config.Config) App {
	return NewAppWithServices(cfg, Services{})
}

// NewAppWithServices creates the default TUI app model with production or test services.
func NewAppWithServices(cfg config.Config, services Services) App {
	app := App{state: State{Current: screens.SettingsCheckID, Config: cfg, SelectedDatabases: map[string]bool{}, Width: 120, Height: 36}, services: services}
	if err := config.Validate(cfg); err != nil {
		app.state.Current = screens.ConfigEditorID
		app.state.Err = err
		app.state.Status = "Нужно настроить подключение перед первым запуском"
		return app
	}
	app.state.Current = screens.DatabaseListID
	app.state.Status = "Загружаю список баз…"
	return app
}

// Init starts loading the database list when a catalog service is available.
func (a App) Init() tea.Cmd {
	if a.state.Current == screens.DatabaseListID && a.services.Catalog != nil && len(a.state.Databases) == 0 {
		return loadDatabasesCmd(a.services.Catalog)
	}
	return nil
}

// Update handles global navigation messages.
//
//nolint:gocyclo // Central Bubble Tea dispatcher intentionally maps message types explicitly.
func (a App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {
	case tea.WindowSizeMsg:
		a.state.Width = m.Width
		a.state.Height = m.Height
		return a, nil
	case SettingsLoadedMsg:
		return a.onSettings(m)
	case DatabasesLoadedMsg:
		return a.onDatabasesLoaded(m), nil
	case syncStartedMsg:
		a.state.ProgressEvents = m.events
		a.state.SyncDone = m.done
		return a, tea.Batch(waitSyncProgressCmd(m.events, m.done), progressTickCmd())
	case syncProgressMsg:
		a.state.ProgressEvent = m.Event
		a.state.Progress.Apply(m.Event, time.Now())
		a.state.Status = progressStatus(m.Event)
		return a, waitSyncProgressCmd(a.state.ProgressEvents, a.state.SyncDone)
	case progressTickMsg:
		if a.state.Running && a.state.Current == screens.ProgressID {
			a.state.Progress.Tick(m.Time)
			return a, progressTickCmd()
		}
		return a, nil
	case SyncFinishedMsg:
		a.state.Running = false
		a.state.Current = screens.ResultID
		a.state.ActiveTab = 0
		a.state.Err = m.Err
		a.state.Result = m.Result
		a.state.Results = m.Results
		if m.Result != nil {
			a.state.Status = fmt.Sprintf("Готово: %s", m.Result.Duration())
		}
		return a, nil
	case tea.KeyMsg:
		return a.onKey(m)
	case tea.MouseMsg:
		return a.onMouse(m)
	default:
		return a, nil
	}
}

// View renders the current app shell.
func (a App) View() string {
	body := a.screenBody()
	if isStyledScreen(a.state.Current) {
		return zone.Scan(body)
	}
	if a.state.Status != "" {
		body += "\n\n" + a.state.Status
	}
	if a.state.Err != nil {
		body += "\nОшибка: " + a.state.Err.Error()
	}
	return zone.Scan(body)
}

func (a App) screenBody() string {
	switch a.state.Current {
	case screens.ConfigEditorID:
		return screens.NewConfigEditor(a.state.Config, screens.WizardMode, nil).View() + "\n\nПодсказка: заполните конфиг через `pgsync config` или TOML-файл."
	case screens.MainMenuID:
		return screens.MainMenu(a.state.MenuIndex).View()
	case screens.DatabaseListID:
		return screens.DatabaseList(a.state.Databases, a.state.Err, screens.DatabaseListOptions{SelectedIndex: a.state.DatabaseIndex, Checked: a.state.SelectedDatabases, Width: a.state.Width, Height: a.state.Height, Status: a.state.Status, Config: a.state.Config}).View()
	case screens.ConfirmPlanID:
		return screens.ConfirmPlan(a.planReviewOptions()).View()
	case screens.ProgressID:
		snapshot := a.state.Progress.Snapshot(a.state.Config, a.state.Width)
		snapshot.Header.Height = a.state.Height
		snapshot.Tab = a.state.ActiveTab
		return screens.ProgressDashboard(snapshot).View()
	case screens.ResultID:
		return screens.Result(a.aggregateResult(), screens.ResultOptions{Header: screens.HeaderOptions{Config: a.state.Config, Database: a.headerDatabase(), Width: a.state.Width, Height: a.state.Height}, Tab: a.state.ActiveTab, Tables: a.state.Progress.TableResults}).View()
	default:
		return fmt.Sprintf("Экран: %s", a.state.Current)
	}
}

// State returns a copy of current state for tests and screen adapters.
func (a App) State() State { return a.state }

func (a App) onSettings(msg SettingsLoadedMsg) (App, tea.Cmd) {
	if msg.Err != nil {
		a.state.Current = screens.ConfigEditorID
		a.state.Err = msg.Err
		a.state.Status = "Нужно настроить подключение перед первым запуском"
		return a, nil
	}
	if err := config.Validate(msg.Config); err != nil {
		a.state.Current = screens.ConfigEditorID
		a.state.Err = err
		a.state.Status = "Конфиг неполный, проверьте поля"
		return a, nil
	}
	a.state.Config = msg.Config
	a.state.Current = screens.DatabaseListID
	a.state.Err = nil
	a.state.Status = "Загружаю список баз…"
	if a.services.Catalog != nil {
		return a, loadDatabasesCmd(a.services.Catalog)
	}
	return a, nil
}

func (a App) onDatabasesLoaded(msg DatabasesLoadedMsg) App {
	sort.SliceStable(msg.Databases, func(i, j int) bool {
		return msg.Databases[i].SizeBytes > msg.Databases[j].SizeBytes
	})
	a.state.Databases = msg.Databases
	a.state.Err = msg.Err
	if msg.Err != nil {
		a.state.Status = "Не удалось загрузить базы"
		return a
	}
	if len(msg.Databases) == 0 {
		a.state.Status = "Базы не найдены"
		return a
	}
	a.state.DatabaseIndex = clampIndex(a.state.DatabaseIndex, len(msg.Databases))
	a.state.Status = fmt.Sprintf("Loaded %d remote databases", len(msg.Databases))
	return a
}

//nolint:gocyclo // Central Bubble Tea router keeps screen dispatch explicit.
func (a App) onKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if a.state.Current == screens.MainMenuID {
		return a.onMainMenuKey(msg)
	}
	if a.state.Current == screens.DatabaseListID {
		return a.onDatabaseListKey(msg)
	}
	if a.state.Current == screens.ConfirmPlanID {
		return a.onConfirmKey(msg)
	}
	if a.state.Current == screens.ProgressID {
		return a.onProgressKey(msg)
	}
	if a.state.Current == screens.ResultID {
		return a.onResultKey(msg)
	}

	switch GlobalKeyAction(msg) {
	case KeyOpenConfig:
		a.state.Current = screens.ConfigEditorID
		a.state.Status = "Настройки"
		return a, tea.Quit
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

//nolint:gocognit,gocyclo // Mirrors keyboard UX from dbsync with multiple explicit shortcuts.
func (a App) onDatabaseListKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		a.state.DatabaseIndex = clampIndex(a.state.DatabaseIndex-1, len(a.state.Databases))
	case "down", "j", "tab":
		a.state.DatabaseIndex = clampIndex(a.state.DatabaseIndex+1, len(a.state.Databases))
	case "space", " ":
		if db, ok := a.currentDatabase(); ok {
			if a.state.SelectedDatabases == nil {
				a.state.SelectedDatabases = map[string]bool{}
			}
			a.state.SelectedDatabases[db.Name] = !a.state.SelectedDatabases[db.Name]
			if !a.state.SelectedDatabases[db.Name] {
				delete(a.state.SelectedDatabases, db.Name)
			}
		}
	case "a":
		if a.state.SelectedDatabases == nil {
			a.state.SelectedDatabases = map[string]bool{}
		}
		for _, db := range a.state.Databases {
			a.state.SelectedDatabases[db.Name] = true
		}
	case "c":
		a.state.SelectedDatabases = map[string]bool{}
	case "r":
		a.state.Status = "Loading remote databases..."
		a.state.Err = nil
		if a.services.Catalog != nil {
			return a, loadDatabasesCmd(a.services.Catalog)
		}
	case "s":
		return a.openSettings()
	case "q", "ctrl+c":
		a.state.Quit = true
		return a, tea.Quit
	case "y", "enter", "right", "l":
		if db, ok := a.currentDatabase(); ok && len(a.state.SelectedDatabases) == 0 {
			if a.state.SelectedDatabases == nil {
				a.state.SelectedDatabases = map[string]bool{}
			}
			a.state.SelectedDatabases[db.Name] = true
		}
		if len(a.state.SelectedDatabases) > 0 {
			a.state.Current = screens.ConfirmPlanID
		}
	}
	return a, nil
}

func (a App) onConfirmKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "left", "h":
		a.state.Current = screens.DatabaseListID
	case "s":
		return a.openSettings()
	case "enter", "y":
		queue := a.selectedDatabaseList()
		if len(queue) == 0 {
			a.state.Current = screens.DatabaseListID
			a.state.Status = "Выберите хотя бы одну базу"
			return a, nil
		}
		a.state.Current = screens.ProgressID
		a.state.Running = true
		a.state.ActiveTab = 0
		a.state.Status = "Sync queued"
		now := time.Now()
		a.state.Progress = NewLiveProgressForQueue(queue, now)
		a.state.ProgressEvent = engine.Event{Name: engine.EventSyncStart, Stage: "planning", Database: queue[0].Name, Time: now, Engine: a.state.Config.Runtime.Engine}
		a.state.Progress.Apply(a.state.ProgressEvent, now)
		return a, a.startSyncCmd(queue)
	case "q", "ctrl+c":
		a.state.Quit = true
		return a, tea.Quit
	}
	return a, nil
}

func (a App) onProgressKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "tab", "right", "l":
		a.state.ActiveTab = (a.state.ActiveTab + 1) % 2
	case "shift+tab", "left", "h":
		a.state.ActiveTab = (a.state.ActiveTab + 1) % 2
	case "q", "ctrl+c":
		a.state.Quit = true
		return a, tea.Quit
	}
	return a, nil
}

func (a App) onResultKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "tab", "right", "l":
		a.state.ActiveTab = (a.state.ActiveTab + 1) % 3
	case "shift+tab", "left", "h":
		a.state.ActiveTab = (a.state.ActiveTab + 2) % 3
	case "enter", "q", "esc", "ctrl+c":
		a.state.Quit = true
		return a, tea.Quit
	case "b":
		a.state.Current = screens.DatabaseListID
	}
	return a, nil
}

//nolint:gocyclo // Menu shortcuts are kept in one small dispatcher for readability.
func (a App) onMainMenuKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	itemsCount := 3
	switch msg.String() {
	case "up", "k":
		a.state.MenuIndex = (a.state.MenuIndex - 1 + itemsCount) % itemsCount
	case "down", "j", "tab":
		a.state.MenuIndex = (a.state.MenuIndex + 1) % itemsCount
	case "s":
		a.state.MenuIndex = 1
		return a.openSettings()
	case "q", "ctrl+c":
		if a.state.Running {
			a.state.Status = "Синхронизация выполняется; подтвердите отмену"
			return a, nil
		}
		a.state.Quit = true
		return a, tea.Quit
	case " ":
		a.state.Status = "Пауза/продолжить"
	case "enter":
		switch a.state.MenuIndex {
		case 0:
			a.state.Current = screens.DatabaseListID
		case 1:
			return a.openSettings()
		case 2:
			a.state.Quit = true
			return a, tea.Quit
		}
	}
	return a, nil
}

func (a App) openSettings() (tea.Model, tea.Cmd) {
	a.state.Current = screens.ConfigEditorID
	a.state.Status = "Настройки"
	return a, tea.Quit
}

func loadDatabasesCmd(catalog CatalogService) tea.Cmd {
	return func() tea.Msg {
		databases, err := catalog.ListDatabases(context.Background())
		return DatabasesLoadedMsg{Databases: databases, Err: err}
	}
}

func (a App) currentDatabase() (models.Database, bool) {
	if len(a.state.Databases) == 0 {
		return models.Database{}, false
	}
	index := clampIndex(a.state.DatabaseIndex, len(a.state.Databases))
	return a.state.Databases[index], true
}

// selectedDatabaseList returns selected DBs in display order with their stats.
func (a App) selectedDatabaseList() []models.Database {
	if len(a.state.SelectedDatabases) == 0 {
		return nil
	}
	out := make([]models.Database, 0, len(a.state.SelectedDatabases))
	for _, db := range a.state.Databases {
		if a.state.SelectedDatabases[db.Name] {
			out = append(out, db)
		}
	}
	return out
}

// planReviewOptions builds the multi-DB plan review screen options from current state.
func (a App) planReviewOptions() screens.PlanReviewOptions {
	queue := a.selectedDatabaseList()
	header := screens.HeaderOptions{Config: a.state.Config, Width: a.state.Width, Height: a.state.Height}
	if len(queue) > 0 {
		header.Database = queue[0].Name
	}
	return screens.PlanReviewOptions{
		Header:           header,
		Databases:        queue,
		Engine:           a.state.Config.Runtime.Engine,
		Threads:          a.state.Config.Runtime.Threads,
		UseSystemPgtools: a.state.Config.Runtime.UseSystemPgtools,
	}
}

func (a App) headerDatabase() string {
	if a.state.Result != nil && a.state.Result.Database != "" {
		return a.state.Result.Database
	}
	if len(a.state.Results) > 0 {
		return fmt.Sprintf("%d databases", len(a.state.Results))
	}
	return a.state.Config.Runtime.DefaultDatabase
}

// aggregateResult combines per-DB results into a single SyncResult for the report screen.
func (a App) aggregateResult() *models.SyncResult {
	if len(a.state.Results) <= 1 {
		return a.state.Result
	}
	combined := &models.SyncResult{Stages: map[string]time.Duration{}}
	for i, r := range a.state.Results {
		if r == nil {
			continue
		}
		if i == 0 || r.StartedAt.Before(combined.StartedAt) {
			combined.StartedAt = r.StartedAt
		}
		if r.FinishedAt.After(combined.FinishedAt) {
			combined.FinishedAt = r.FinishedAt
		}
		combined.BytesCopied += r.BytesCopied
		combined.RowsCopied += r.RowsCopied
		combined.TablesCopied += r.TablesCopied
		for stage, d := range r.Stages {
			combined.Stages[stage] += d
		}
		if r.Err != nil && combined.Err == nil {
			combined.Err = r.Err
		}
	}
	combined.Database = fmt.Sprintf("%d databases", len(a.state.Results))
	return combined
}

//nolint:gocognit,gocyclo // Sequential planner+executor loop with per-DB error handling stays in one place for readability.
func (a App) startSyncCmd(queue []models.Database) tea.Cmd {
	planner := a.services.Planner
	executor := a.services.Executor
	cfg := a.state.Config
	return func() tea.Msg {
		events := make(chan engine.Event, 256)
		done := make(chan SyncFinishedMsg, 1)
		go func() {
			defer close(events)
			if executor == nil {
				done <- SyncFinishedMsg{Err: fmt.Errorf("sync executor is not configured")}
				return
			}
			ctx := context.Background()
			results := make([]*models.SyncResult, 0, len(queue))
			var lastResult *models.SyncResult
			var lastErr error
			for _, db := range queue {
				select {
				case events <- engine.Event{Time: time.Now(), Name: engine.EventSyncStart, Stage: "planning", Database: db.Name, Engine: cfg.Runtime.Engine}:
				default:
				}
				var plan *models.SyncPlan
				if planner != nil {
					p, err := planner.Plan(ctx, engine.PlanOptions{
						Remote:           cfg.Remote,
						Local:            cfg.Local,
						Database:         db.Name,
						Threads:          cfg.Runtime.Threads,
						Mode:             engine.Mode(cfg.Runtime.Engine),
						UseSystemPgtools: cfg.Runtime.UseSystemPgtools,
					})
					if err != nil {
						lastErr = err
						break
					}
					plan = p
				}
				dbName := db.Name
				observer := engine.ObserverFunc(func(ctx context.Context, event engine.Event) {
					if event.Database == "" {
						event.Database = dbName
					}
					select {
					case events <- event:
					case <-ctx.Done():
					}
				})
				result, err := executor.Execute(ctx, plan, observer)
				if result != nil {
					results = append(results, result)
					lastResult = result
				}
				if err != nil {
					lastErr = err
					break
				}
			}
			done <- SyncFinishedMsg{Result: lastResult, Results: results, Err: lastErr}
		}()
		return syncStartedMsg{events: events, done: done}
	}
}

func waitSyncProgressCmd(events <-chan engine.Event, done <-chan SyncFinishedMsg) tea.Cmd {
	return func() tea.Msg {
		select {
		case event, ok := <-events:
			if ok {
				return syncProgressMsg{Event: event}
			}
			return <-done
		case msg := <-done:
			return msg
		}
	}
}

func progressStatus(event engine.Event) string {
	if event.Table != "" {
		return fmt.Sprintf("%s %.1f%%", event.Table, event.Percent)
	}
	if event.Stage != "" {
		return event.Stage
	}
	return event.Name
}

func clampIndex(index, length int) int {
	if length <= 0 {
		return 0
	}
	if index < 0 {
		return length - 1
	}
	if index >= length {
		return 0
	}
	return index
}

func isStyledScreen(id screens.ID) bool {
	switch id {
	case screens.DatabaseListID, screens.ConfirmPlanID, screens.ProgressID, screens.ResultID:
		return true
	default:
		return false
	}
}

func nextScreen(id screens.ID) screens.ID {
	switch id {
	case screens.SettingsCheckID:
		return screens.MainMenuID
	case screens.MainMenuID:
		return screens.DatabaseListID
	case screens.DatabaseListID:
		return screens.ConfirmPlanID
	case screens.ConfirmPlanID:
		return screens.ProgressID
	case screens.ProgressID:
		return screens.ResultID
	default:
		return id
	}
}

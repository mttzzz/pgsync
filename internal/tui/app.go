package tui

import (
	"context"
	"fmt"
	"sort"

	tea "github.com/charmbracelet/bubbletea"

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
	Tables            []models.Table
	SelectedTables    map[string]bool
	TableIndex        int
	TablesLoading     bool
	TablesErr         error
	Result            *models.SyncResult
	ProgressEvent     engine.Event
	ProgressEvents    <-chan engine.Event
	SyncDone          <-chan SyncFinishedMsg
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
	app := App{state: State{Current: screens.SettingsCheckID, Config: cfg, SelectedDatabases: map[string]bool{}, SelectedTables: map[string]bool{}, Width: 120, Height: 36}, services: services}
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
	case TablesLoadedMsg:
		return a.onTablesLoaded(m), nil
	case syncStartedMsg:
		a.state.ProgressEvents = m.events
		a.state.SyncDone = m.done
		return a, waitSyncProgressCmd(m.events, m.done)
	case syncProgressMsg:
		a.state.ProgressEvent = m.Event
		a.state.Status = progressStatus(m.Event)
		return a, waitSyncProgressCmd(a.state.ProgressEvents, a.state.SyncDone)
	case SyncFinishedMsg:
		a.state.Running = false
		a.state.Current = screens.ResultID
		a.state.Err = m.Err
		a.state.Result = m.Result
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
	if isStyledScreen(a.state.Current) {
		return body
	}
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
		return screens.MainMenu(a.state.MenuIndex).View()
	case screens.DatabaseListID:
		return screens.DatabaseList(a.state.Databases, a.state.Err, screens.DatabaseListOptions{SelectedIndex: a.state.DatabaseIndex, Checked: a.state.SelectedDatabases, Width: a.state.Width, Height: a.state.Height, Status: a.state.Status}).View()
	case screens.TablesPickID:
		return screens.TablesPick(a.state.Tables, screens.TableListOptions{Database: a.state.Config.Runtime.DefaultDatabase, SelectedIndex: a.state.TableIndex, Checked: a.state.SelectedTables, Loading: a.state.TablesLoading, Err: a.state.TablesErr, Width: a.state.Width, Height: a.state.Height, Status: a.state.Status}).View()
	case screens.ConfirmPlanID:
		return screens.ConfirmPlan(a.currentPlan()).View()
	case screens.ProgressID:
		return screens.Progress(progressStage(a.state.ProgressEvent), a.state.ProgressEvent.Percent).View()
	case screens.ResultID:
		return screens.Result(a.state.Result).View()
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

func (a App) onTablesLoaded(msg TablesLoadedMsg) App {
	a.state.TablesLoading = false
	a.state.TablesErr = msg.Err
	if msg.Err != nil {
		a.state.Status = "Не удалось загрузить таблицы"
		return a
	}
	a.state.Tables = msg.Tables
	a.state.SelectedTables = map[string]bool{}
	for _, table := range msg.Tables {
		a.state.SelectedTables[tableKey(table)] = true
	}
	a.state.TableIndex = clampIndex(a.state.TableIndex, len(msg.Tables))
	a.state.Status = fmt.Sprintf("Loaded %d tables from %s", len(msg.Tables), a.state.Config.Runtime.DefaultDatabase)
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
	if a.state.Current == screens.TablesPickID {
		return a.onTablesKey(msg)
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
	case "y":
		if len(a.state.SelectedDatabases) > 0 {
			a.state.Current = screens.ConfirmPlanID
		}
	case "enter", "right", "l":
		if db, ok := a.currentDatabase(); ok {
			a.state.Config.Runtime.DefaultDatabase = db.Name
			a.state.Current = screens.TablesPickID
			a.state.Tables = nil
			a.state.SelectedTables = map[string]bool{}
			a.state.TableIndex = 0
			a.state.TablesErr = nil
			a.state.TablesLoading = true
			a.state.Status = "Loading tables from " + db.Name + "..."
			if a.state.SelectedDatabases == nil {
				a.state.SelectedDatabases = map[string]bool{}
			}
			a.state.SelectedDatabases[db.Name] = true
			if a.services.Catalog != nil {
				return a, loadTablesCmd(a.services.Catalog, db.Name)
			}
		}
	}
	return a, nil
}

//nolint:gocyclo // Table picker intentionally maps many single-key actions.
func (a App) onTablesKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "left", "h":
		a.state.Current = screens.DatabaseListID
	case "up", "k":
		a.state.TableIndex = clampIndex(a.state.TableIndex-1, len(a.state.Tables))
	case "down", "j", "tab":
		a.state.TableIndex = clampIndex(a.state.TableIndex+1, len(a.state.Tables))
	case "space", " ":
		if table, ok := a.currentTable(); ok {
			if a.state.SelectedTables == nil {
				a.state.SelectedTables = map[string]bool{}
			}
			key := tableKey(table)
			a.state.SelectedTables[key] = !a.state.SelectedTables[key]
			if !a.state.SelectedTables[key] {
				delete(a.state.SelectedTables, key)
			}
		}
	case "a":
		a.state.SelectedTables = map[string]bool{}
		for _, table := range a.state.Tables {
			a.state.SelectedTables[tableKey(table)] = true
		}
	case "c":
		a.state.SelectedTables = map[string]bool{}
	case "r":
		database := a.state.Config.Runtime.DefaultDatabase
		if database != "" && a.services.Catalog != nil {
			a.state.TablesLoading = true
			a.state.TablesErr = nil
			a.state.Status = "Reloading tables from " + database + "..."
			return a, loadTablesCmd(a.services.Catalog, database)
		}
	case "s":
		return a.openSettings()
	case "q", "ctrl+c":
		a.state.Quit = true
		return a, tea.Quit
	case "y", "enter":
		a.state.Current = screens.ConfirmPlanID
	}
	return a, nil
}

func (a App) onConfirmKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "left", "h":
		if a.state.Config.Runtime.DefaultDatabase != "" {
			a.state.Current = screens.TablesPickID
		} else {
			a.state.Current = screens.DatabaseListID
		}
	case "enter", "y":
		a.state.Current = screens.ProgressID
		a.state.Running = true
		a.state.Status = "Sync queued"
		a.state.ProgressEvent = engine.Event{Name: engine.EventSyncStart, Stage: "planning", Database: a.state.Config.Runtime.DefaultDatabase}
		return a, a.startSyncCmd()
	case "q", "ctrl+c":
		a.state.Quit = true
		return a, tea.Quit
	}
	return a, nil
}

func (a App) onProgressKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		a.state.Quit = true
		return a, tea.Quit
	}
	return a, nil
}

func (a App) onResultKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
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

func loadTablesCmd(catalog CatalogService, database string) tea.Cmd {
	return func() tea.Msg {
		tables, err := catalog.ListTables(context.Background(), database)
		return TablesLoadedMsg{Tables: tables, Err: err}
	}
}

func (a App) currentDatabase() (models.Database, bool) {
	if len(a.state.Databases) == 0 {
		return models.Database{}, false
	}
	index := clampIndex(a.state.DatabaseIndex, len(a.state.Databases))
	return a.state.Databases[index], true
}

func (a App) currentTable() (models.Table, bool) {
	if len(a.state.Tables) == 0 {
		return models.Table{}, false
	}
	index := clampIndex(a.state.TableIndex, len(a.state.Tables))
	return a.state.Tables[index], true
}

func (a App) startSyncCmd() tea.Cmd {
	plan := a.currentPlan()
	planner := a.services.Planner
	executor := a.services.Executor
	cfg := a.state.Config
	tables := selectedTableNames(a.state.Tables, a.state.SelectedTables)
	return func() tea.Msg {
		events := make(chan engine.Event, 128)
		done := make(chan SyncFinishedMsg, 1)
		go func() {
			defer close(events)
			if executor == nil {
				done <- SyncFinishedMsg{Result: nil, Err: fmt.Errorf("sync executor is not configured")}
				return
			}
			ctx := context.Background()
			if planner != nil {
				var err error
				plan, err = planner.Plan(ctx, engine.PlanOptions{
					Remote:           cfg.Remote,
					Local:            cfg.Local,
					Database:         cfg.Runtime.DefaultDatabase,
					Tables:           tables,
					Threads:          cfg.Runtime.Threads,
					Mode:             engine.Mode(cfg.Runtime.Engine),
					UseSystemPgtools: cfg.Runtime.UseSystemPgtools,
					Yes:              true,
				})
				if err != nil {
					done <- SyncFinishedMsg{Err: err}
					return
				}
			}
			observer := engine.ObserverFunc(func(ctx context.Context, event engine.Event) {
				select {
				case events <- event:
				case <-ctx.Done():
				}
			})
			result, err := executor.Execute(ctx, plan, observer)
			done <- SyncFinishedMsg{Result: result, Err: err}
		}()
		return syncStartedMsg{events: events, done: done}
	}
}

func selectedTableNames(tables []models.Table, selected map[string]bool) []string {
	if len(tables) == 0 || len(selected) == 0 {
		return nil
	}
	names := make([]string, 0, len(selected))
	for _, table := range tables {
		if selected[tableKey(table)] {
			names = append(names, table.Schema+"."+table.Name)
		}
	}
	return names
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

func progressStage(event engine.Event) string {
	if event.Table != "" {
		return event.Table
	}
	if event.Stage != "" {
		return event.Stage
	}
	if event.Name != "" {
		return event.Name
	}
	return "waiting"
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

func (a App) currentPlan() *models.SyncPlan {
	database := a.state.Config.Runtime.DefaultDatabase
	if database == "" {
		if db, ok := a.currentDatabase(); ok {
			database = db.Name
		}
	}
	if database == "" {
		return nil
	}
	selectedTables := make([]models.Table, 0, len(a.state.Tables))
	if len(a.state.Tables) > 0 {
		for _, table := range a.state.Tables {
			if a.state.SelectedTables == nil || a.state.SelectedTables[tableKey(table)] {
				selectedTables = append(selectedTables, table)
			}
		}
	}
	return &models.SyncPlan{Database: database, Tables: selectedTables, Threads: a.state.Config.Runtime.Threads, Engine: a.state.Config.Runtime.Engine, Remote: a.state.Config.Remote, Local: a.state.Config.Local}
}

func tableKey(table models.Table) string {
	return table.Schema + "." + table.Name
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
	case screens.DatabaseListID, screens.TablesPickID, screens.ConfirmPlanID, screens.ProgressID, screens.ResultID:
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

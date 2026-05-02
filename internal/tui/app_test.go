package tui

import (
	"context"
	"errors"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mttzzz/pgsync/internal/config"
	"github.com/mttzzz/pgsync/internal/models"
	"github.com/mttzzz/pgsync/internal/tui/screens"
)

func TestAppSettingsTransitions(t *testing.T) {
	t.Parallel()
	cfg := validCfg()
	app := App{state: State{Current: screens.SettingsCheckID, Config: config.Defaults()}}
	assert.Nil(t, app.Init())
	assert.Equal(t, screens.SettingsCheckID, app.State().Current)

	model, cmd := app.Update(SettingsLoadedMsg{Config: cfg})
	require.Nil(t, cmd)
	app = model.(App)
	assert.Equal(t, screens.DatabaseListID, app.State().Current)
	assert.Contains(t, app.View(), "Загружаю")

	model, _ = app.Update(SettingsLoadedMsg{Err: errors.New("missing")})
	app = model.(App)
	assert.Equal(t, screens.ConfigEditorID, app.State().Current)
	assert.Contains(t, app.View(), "первым запуском")

	bad := cfg
	bad.Remote.Host = ""
	model, _ = app.Update(SettingsLoadedMsg{Config: bad})
	app = model.(App)
	assert.Equal(t, screens.ConfigEditorID, app.State().Current)
	assert.Contains(t, app.View(), "Конфиг неполный")
}

func TestNewAppInitialRouting(t *testing.T) {
	t.Parallel()
	invalid := NewApp(config.Defaults())
	assert.Equal(t, screens.ConfigEditorID, invalid.State().Current)
	assert.Contains(t, invalid.View(), "Подсказка")

	valid := NewApp(validCfg())
	assert.Equal(t, screens.DatabaseListID, valid.State().Current)
	assert.Contains(t, valid.View(), "Database Queue Builder")
}

func TestAppKeysAndMessages(t *testing.T) {
	t.Parallel()
	app := NewApp(validCfg())
	model, _ := app.Update(DatabasesLoadedMsg{Databases: []models.Database{{Name: "alpha"}, {Name: "beta"}}})
	app = model.(App)
	assert.Contains(t, app.View(), "alpha")

	model, _ = app.Update(tea.KeyMsg{Type: tea.KeyDown})
	app = model.(App)
	assert.Equal(t, 1, app.State().DatabaseIndex)
	model, _ = app.Update(key("enter"))
	app = model.(App)
	assert.Equal(t, screens.TablesPickID, app.State().Current)
	assert.Equal(t, "beta", app.State().Config.Runtime.DefaultDatabase)

	model, _ = app.Update(key("s"))
	app = model.(App)
	assert.Equal(t, screens.ConfigEditorID, app.State().Current)
	model, _ = app.Update(key("esc"))
	app = model.(App)
	assert.Equal(t, screens.MainMenuID, app.State().Current)
	model, _ = app.Update(key(" "))
	app = model.(App)
	assert.Contains(t, app.State().Status, "Пауза")

	app.state.Running = true
	model, cmd := app.Update(key("q"))
	app = model.(App)
	assert.Nil(t, cmd)
	assert.False(t, app.State().Quit)
	assert.Contains(t, app.State().Status, "подтвердите")

	app.state.Running = false
	model, cmd = app.Update(key("q"))
	app = model.(App)
	assert.NotNil(t, cmd)
	assert.True(t, app.State().Quit)

	res := &models.SyncResult{StartedAt: time.Unix(0, 0), FinishedAt: time.Unix(1, 0)}
	model, _ = app.Update(SyncFinishedMsg{Result: res})
	app = model.(App)
	assert.Equal(t, screens.ResultID, app.State().Current)
	assert.Contains(t, app.View(), "Sync Report")

	model, _ = app.Update(struct{}{})
	assert.IsType(t, App{}, model)
}

func TestGlobalKeyAction(t *testing.T) {
	t.Parallel()
	assert.Equal(t, KeyOpenConfig, GlobalKeyAction(key("s")))
	assert.Equal(t, KeyBack, GlobalKeyAction(key("esc")))
	assert.Equal(t, KeyQuit, GlobalKeyAction(key("q")))
	assert.Equal(t, KeyQuit, GlobalKeyAction(key("ctrl+c")))
	assert.Equal(t, KeyConfirm, GlobalKeyAction(key("enter")))
	assert.Equal(t, KeyTogglePause, GlobalKeyAction(key(" ")))
	assert.Equal(t, KeyNone, GlobalKeyAction(key("x")))
}

func TestScreenBody(t *testing.T) {
	t.Parallel()
	app := NewApp(validCfg())
	for _, id := range []screens.ID{
		screens.ConfigEditorID,
		screens.MainMenuID,
		screens.DatabaseListID,
		screens.TablesPickID,
		screens.ConfirmPlanID,
		screens.ProgressID,
		screens.ResultID,
		screens.SettingsCheckID,
	} {
		app.state.Current = id
		assert.NotEmpty(t, app.screenBody())
	}
}

func TestAppLoadsDatabasesThroughCatalogService(t *testing.T) {
	t.Parallel()
	catalog := &fakeCatalogService{databases: []models.Database{{Name: "app", SizeBytes: 1024, Owner: "postgres"}}}
	app := NewAppWithServices(validCfg(), Services{Catalog: catalog})
	cmd := app.Init()
	require.NotNil(t, cmd)

	msg := cmd()
	model, updateCmd := app.Update(msg)
	require.Nil(t, updateCmd)
	app = model.(App)
	assert.True(t, catalog.listedDatabases)
	assert.Contains(t, app.View(), "app")
	assert.Contains(t, app.View(), "Database Queue Builder")
	assert.Contains(t, app.State().Status, "Loaded")
}

func TestNextScreen(t *testing.T) {
	t.Parallel()
	assert.Equal(t, screens.MainMenuID, nextScreen(screens.SettingsCheckID))
	assert.Equal(t, screens.DatabaseListID, nextScreen(screens.MainMenuID))
	assert.Equal(t, screens.ConfirmPlanID, nextScreen(screens.DatabaseListID))
	assert.Equal(t, screens.ConfirmPlanID, nextScreen(screens.TablesPickID))
	assert.Equal(t, screens.ProgressID, nextScreen(screens.ConfirmPlanID))
	assert.Equal(t, screens.ResultID, nextScreen(screens.ProgressID))
	assert.Equal(t, screens.ResultID, nextScreen(screens.ResultID))
}

func TestQueue(t *testing.T) {
	t.Parallel()
	first := &models.SyncPlan{Database: "a"}
	second := &models.SyncPlan{Database: "b"}
	q := Queue{}.Enqueue(first, second)
	assert.Equal(t, 2, q.Len())
	q = q.Pause().StartNext()
	assert.Nil(t, q.Current)
	q = q.Resume().StartNext()
	assert.Equal(t, first, q.Current)
	assert.Equal(t, 1, q.Len())
	q = q.StartNext()
	assert.Equal(t, first, q.Current)
	q = q.Complete(&models.SyncResult{Database: "a"})
	assert.Nil(t, q.Current)
	assert.Len(t, q.Results, 1)
	q = q.StartNext()
	assert.Equal(t, second, q.Current)
	q = q.Complete(nil)
	assert.Len(t, q.Results, 1)
}

func key(s string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

type fakeCatalogService struct {
	databases       []models.Database
	listedDatabases bool
}

func (f *fakeCatalogService) ListDatabases(ctx context.Context) ([]models.Database, error) {
	f.listedDatabases = true
	return f.databases, nil
}

func (f *fakeCatalogService) ListTables(ctx context.Context, database string) ([]models.Table, error) {
	return nil, nil
}

func validCfg() config.Config {
	cfg := config.Defaults()
	cfg.Remote.Host = "prod"
	cfg.Remote.User = "u"
	cfg.Local.Host = "localhost"
	cfg.Local.User = "postgres"
	return cfg
}

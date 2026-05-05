package e2e_test

import (
	"context"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mttzzz/pgsync/internal/config"
	"github.com/mttzzz/pgsync/internal/models"
	"github.com/mttzzz/pgsync/internal/tui"
	"github.com/mttzzz/pgsync/internal/tui/screens"
)

func TestTUIDatabaseListFlow(t *testing.T) {
	t.Parallel()

	catalog := &e2eCatalog{
		databases: []models.Database{
			{Name: "analytics", SizeBytes: 10 * 1024 * 1024, Owner: "postgres"},
			{Name: "billing", SizeBytes: 2 * 1024 * 1024, Owner: "app"},
		},
	}
	app := tui.NewAppWithServices(e2eConfig(), tui.Services{Catalog: catalog})

	cmd := app.Init()
	require.NotNil(t, cmd)
	msg := cmd()
	model, updateCmd := app.Update(msg)
	require.Nil(t, updateCmd)
	app = model.(tui.App)

	assert.True(t, catalog.listedDatabases)
	assert.Equal(t, screens.DatabaseListID, app.State().Current)
	assert.Contains(t, app.View(), "Database Queue Builder")
	assert.Contains(t, app.View(), "▸")
	assert.Contains(t, app.View(), "analytics")
	assert.Contains(t, app.View(), "billing")
	assert.Contains(t, app.View(), "Space")

	model, updateCmd = app.Update(tea.KeyMsg{Type: tea.KeyDown})
	require.Nil(t, updateCmd)
	app = model.(tui.App)
	assert.Equal(t, 1, app.State().DatabaseIndex)
	assert.Contains(t, app.View(), "billing")

	model, updateCmd = app.Update(tea.KeyMsg{Type: tea.KeyEnter})
	require.Nil(t, updateCmd)
	app = model.(tui.App)
	assert.Equal(t, screens.ConfirmPlanID, app.State().Current)
	assert.True(t, app.State().SelectedDatabases["billing"])
	assert.Contains(t, app.View(), "Plan Review")
	assert.Contains(t, app.View(), "billing")
}

type e2eCatalog struct {
	databases       []models.Database
	listedDatabases bool
}

func (c *e2eCatalog) ListDatabases(ctx context.Context) ([]models.Database, error) {
	c.listedDatabases = true
	return c.databases, nil
}

func (c *e2eCatalog) ListTables(ctx context.Context, database string) ([]models.Table, error) {
	return nil, nil
}

func e2eConfig() config.Config {
	cfg := config.Defaults()
	cfg.Remote.Host = "prod.example.local"
	cfg.Remote.User = "readonly"
	cfg.Local.Host = "localhost"
	cfg.Local.User = "postgres"
	return cfg
}

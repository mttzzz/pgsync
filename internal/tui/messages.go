package tui

import (
	"github.com/mttzzz/pgsync/internal/config"
	"github.com/mttzzz/pgsync/internal/models"
)

// SettingsLoadedMsg reports settings-check completion.
type SettingsLoadedMsg struct {
	Config config.Config
	Err    error
}

// DatabasesLoadedMsg reports database-list loading completion.
type DatabasesLoadedMsg struct {
	Databases []models.Database
	Err       error
}

// TablesLoadedMsg reports table-list loading completion.
type TablesLoadedMsg struct {
	Tables []models.Table
	Err    error
}

// SyncFinishedMsg reports sync completion.
type SyncFinishedMsg struct {
	Result *models.SyncResult
	Err    error
}

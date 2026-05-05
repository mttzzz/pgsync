package tui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/mttzzz/pgsync/internal/config"
	"github.com/mttzzz/pgsync/internal/engine"
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

type syncStartedMsg struct {
	events <-chan tea.Msg
	done   <-chan SyncFinishedMsg
}

type syncProgressMsg struct {
	Event engine.Event
}

// dbPlanReadyMsg announces that a DB plan has been built and totals are known.
// Emitted by startSyncCmd before invoking the executor for that DB.
type dbPlanReadyMsg struct {
	Database string
	Tables   []models.Table
	Rows     int64
	Bytes    int64
}

// SyncFinishedMsg reports sync completion.
type SyncFinishedMsg struct {
	Result  *models.SyncResult
	Results []*models.SyncResult
	Err     error
}

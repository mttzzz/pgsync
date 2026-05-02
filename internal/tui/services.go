// Package tui implements the full-screen interactive application.
package tui

import (
	"context"

	"github.com/mttzzz/pgsync/internal/config"
	"github.com/mttzzz/pgsync/internal/engine"
	"github.com/mttzzz/pgsync/internal/models"
)

// ConfigStore is the TUI-facing config persistence port.
type ConfigStore interface {
	Load(ctx context.Context) (config.Config, error)
	Save(ctx context.Context, cfg config.Config) error
	Reset(ctx context.Context) error
}

// ConnectionTester tests configured remote and local connections.
type ConnectionTester interface {
	TestRemote(ctx context.Context, cfg config.Config) error
	TestLocal(ctx context.Context, cfg config.Config) error
}

// CatalogService lists databases and tables for selection screens.
type CatalogService interface {
	ListDatabases(ctx context.Context) ([]models.Database, error)
	ListTables(ctx context.Context, database string) ([]models.Table, error)
}

// Planner plans a database sync.
type Planner interface {
	Plan(ctx context.Context, opts engine.PlanOptions) (*models.SyncPlan, error)
}

// SyncExecutor executes a planned sync.
type SyncExecutor interface {
	Execute(ctx context.Context, plan *models.SyncPlan, observer engine.ProgressObserver) (*models.SyncResult, error)
}

// Services groups TUI dependencies.
type Services struct {
	ConfigStore      ConfigStore
	ConnectionTester ConnectionTester
	Catalog          CatalogService
	Planner          Planner
	Executor         SyncExecutor
}

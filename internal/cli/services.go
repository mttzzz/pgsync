package cli

import (
	"context"

	"github.com/mttzzz/pgsync/internal/config"
	"github.com/mttzzz/pgsync/internal/engine"
	"github.com/mttzzz/pgsync/internal/models"
)

// ConfigStore is the CLI-facing config persistence port.
type ConfigStore interface {
	Load(path string) (config.Config, error)
	Save(path string, cfg config.Config) error
	DefaultPath() (string, error)
	Remove(path string) error
}

// SyncService is the CLI-facing sync engine port.
type SyncService interface {
	Plan(ctx context.Context, opts engine.PlanOptions) (*models.SyncPlan, error)
	Execute(ctx context.Context, plan *models.SyncPlan, observer engine.ProgressObserver) (*models.SyncResult, error)
}

// TUIRunner launches interactive UI flows.
type TUIRunner interface {
	Run(ctx context.Context, mode TUIMode) error
}

// TUIRunnerWithConfig launches interactive UI flows with command-line config path awareness.
type TUIRunnerWithConfig interface {
	RunWithConfig(ctx context.Context, mode TUIMode, configPath string) error
}

// TUIRunnerFunc adapts a function to TUIRunner.
type TUIRunnerFunc func(ctx context.Context, mode TUIMode) error

// Run calls f.
func (f TUIRunnerFunc) Run(ctx context.Context, mode TUIMode) error { return f(ctx, mode) }

// TUIMode selects which interactive flow to launch.
type TUIMode string

const (
	// TUIModeApp launches the default full application.
	TUIModeApp TUIMode = "app"
	// TUIModeConfigEdit launches config editing.
	TUIModeConfigEdit TUIMode = "config-edit"
	// TUIModeConfigReset launches config reset wizard.
	TUIModeConfigReset TUIMode = "config-reset"
)

// Package engine defines stable sync engine contracts, options, and progress events.
package engine

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"strings"

	"github.com/mttzzz/pgsync/internal/config"
	"github.com/mttzzz/pgsync/internal/models"
)

// Mode selects the sync engine implementation.
type Mode string

const (
	// ModeAuto lets the application pick the best available engine.
	ModeAuto Mode = "auto"
	// ModeNative selects the Go-native sync engine.
	ModeNative Mode = "native"
	// ModeExternal selects external PostgreSQL tooling.
	ModeExternal Mode = "external"
)

// PlanOptions contains all inputs needed to build a sync plan.
type PlanOptions struct {
	Remote            config.Connection
	Local             config.Connection
	Database          string
	Tables            []string
	Threads           int
	Mode              Mode
	UseSystemPgtools  bool
	DryRun            bool
	Yes               bool
	ConcurrentIndexes bool
	Analyze           bool
}

// Validate validates and normalizes plan options in place.
func (opts *PlanOptions) Validate() error {
	if opts == nil {
		return errors.New("plan options are required")
	}

	endpoints := []struct {
		name string
		conn config.Connection
	}{
		{name: "remote", conn: opts.Remote},
		{name: "local", conn: opts.Local},
	}
	for _, endpoint := range endpoints {
		if err := validatePlanEndpoint(endpoint.name, endpoint.conn); err != nil {
			return err
		}
	}

	opts.Database = strings.TrimSpace(opts.Database)
	if opts.Database == "" {
		return errors.New("database is required")
	}
	if opts.Threads < 0 {
		return errors.New("threads must be >= 0")
	}
	if opts.Threads == 0 {
		opts.Threads = runtime.NumCPU()
	}
	if !isValidMode(opts.Mode) {
		return fmt.Errorf("engine mode must be auto|native|external, got %q", opts.Mode)
	}

	opts.Tables = normalizeTables(opts.Tables)
	return nil
}

// Engine builds and executes sync plans.
type Engine interface {
	Plan(ctx context.Context, opts PlanOptions) (*models.SyncPlan, error)
	Execute(ctx context.Context, plan *models.SyncPlan, observer ProgressObserver) (*models.SyncResult, error)
}

func validatePlanEndpoint(name string, conn config.Connection) error {
	if err := config.ValidateHost(conn.Host); err != nil {
		return fmt.Errorf("%s host: %w", name, err)
	}
	if err := config.ValidatePort(conn.Port); err != nil {
		return fmt.Errorf("%s port: %w", name, err)
	}
	if err := config.ValidateSSLMode(conn.SSLMode); err != nil {
		return fmt.Errorf("%s ssl_mode: %w", name, err)
	}
	return nil
}

func isValidMode(mode Mode) bool {
	switch mode {
	case ModeAuto, ModeNative, ModeExternal:
		return true
	default:
		return false
	}
}

func normalizeTables(tables []string) []string {
	if len(tables) == 0 {
		return nil
	}

	normalized := make([]string, 0, len(tables))
	seen := make(map[string]struct{}, len(tables))
	for _, table := range tables {
		table = strings.TrimSpace(table)
		if table == "" {
			continue
		}
		if _, ok := seen[table]; ok {
			continue
		}
		seen[table] = struct{}{}
		normalized = append(normalized, table)
	}
	return normalized
}

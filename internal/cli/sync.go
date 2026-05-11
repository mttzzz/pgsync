package cli

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"

	"github.com/spf13/cobra"

	"github.com/mttzzz/pgsync/internal/clock"
	"github.com/mttzzz/pgsync/internal/config"
	"github.com/mttzzz/pgsync/internal/engine"
	"github.com/mttzzz/pgsync/internal/observability"
)

func newSyncCommand(app App, globals *globalFlags) *cobra.Command {
	flags := SyncFlags{}
	cmd := &cobra.Command{
		Use:   "sync [db]",
		Short: "Sync a remote PostgreSQL database to the local target",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			database := ""
			if len(args) > 0 {
				database = args[0]
			}
			return runSync(cmd.Context(), app, globals.overrides(cmd), flags, database)
		},
	}
	cmd.Flags().StringSliceVar(&flags.Tables, "tables", nil, "comma-separated table allow-list")
	cmd.Flags().BoolVar(&flags.DryRun, "dry-run", false, "print the sync plan without executing it")
	cmd.Flags().BoolVar(&flags.Analyze, "analyze", false, "run ANALYZE after copy")
	return cmd
}

func runSync(ctx context.Context, app App, overrides FlagOverrides, syncFlags SyncFlags, db string) error {
	db = strings.TrimSpace(db)
	if db != "" {
		if overrides.Remote.Database == "" {
			overrides.Remote.Database = db
		}
		if overrides.Local.Database == "" {
			overrides.Local.Database = db
		}
	}
	resolveFn := app.ResolveFn
	if resolveFn == nil {
		resolveFn = Resolve
	}
	cfg, err := resolveFn(ctx, overrides)
	if err != nil {
		return err
	}
	opts, err := PlanOptionsFromConfig(cfg, db, syncFlags)
	if err != nil {
		return sanitizeError(err, cfg)
	}
	logger := syncLogger(cfg, overrides, app.Err)
	eng, err := app.EngineFactory(logger)
	if err != nil {
		return sanitizeError(err, cfg)
	}
	plan, err := eng.Plan(ctx, opts)
	if err != nil {
		return sanitizeError(err, cfg)
	}
	plainOpts := plainOptions(overrides)
	if syncFlags.DryRun {
		return PrintPlan(app.Out, plan, plainOpts)
	}
	result, err := eng.Execute(ctx, plan, newSyncObserver(app.Out, overrides))
	if err != nil {
		return sanitizeError(err, cfg)
	}
	if overrides.Output == "json" {
		return nil
	}
	return PrintResult(app.Out, result, plainOpts)
}

func syncLogger(cfg config.Config, overrides FlagOverrides, out io.Writer) *slog.Logger {
	level := cfg.Logging.Level
	if overrides.Quiet {
		level = "error"
	}
	if overrides.Verbose {
		level = "debug"
	}
	logger, _ := observability.NewLogger(observability.Options{Level: level, Format: cfg.Logging.Format, Out: out})
	return logger
}

func newSyncObserver(out io.Writer, overrides FlagOverrides) engine.ProgressObserver {
	if overrides.Output == "json" {
		return NewNDJSONObserver(out, clock.NewSystem())
	}
	return NewPlainObserver(out, plainOptions(overrides))
}

func plainOptions(overrides FlagOverrides) PlainOptions {
	return PlainOptions{Quiet: overrides.Quiet, NoColor: overrides.NoColor, Color: !overrides.NoColor}
}

func sanitizeError(err error, cfg config.Config) error {
	if err == nil {
		return nil
	}
	message := err.Error()
	for _, secret := range []string{cfg.Remote.Password, cfg.Local.Password} {
		if secret != "" {
			message = strings.ReplaceAll(message, secret, "******")
		}
	}
	if message == err.Error() {
		return err
	}
	return errors.New(message)
}

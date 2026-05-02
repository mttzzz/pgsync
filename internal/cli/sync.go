package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"

	"github.com/spf13/cobra"

	"github.com/mttzzz/pgsync/internal/config"
	"github.com/mttzzz/pgsync/internal/engine"
	"github.com/mttzzz/pgsync/internal/models"
	"github.com/mttzzz/pgsync/internal/observability"
)

func newSyncCommand(app App, globals *globalFlags) *cobra.Command {
	flags := SyncFlags{}
	cmd := &cobra.Command{
		Use:   "sync <db>",
		Short: "Sync a remote PostgreSQL database to the local target",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSync(cmd.Context(), app, globals.overrides(cmd), flags, args[0])
		},
	}
	cmd.Flags().StringSliceVar(&flags.Tables, "tables", nil, "comma-separated table allow-list")
	cmd.Flags().BoolVar(&flags.DryRun, "dry-run", false, "print the sync plan without executing it")
	cmd.Flags().BoolVar(&flags.Yes, "yes", false, "confirm destructive target reset")
	cmd.Flags().BoolVar(&flags.Analyze, "analyze", false, "run ANALYZE after copy")
	return cmd
}

func runSync(ctx context.Context, app App, overrides FlagOverrides, syncFlags SyncFlags, db string) error {
	cfg, err := Resolve(ctx, overrides)
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
	if syncFlags.DryRun {
		return printPlan(app.Out, plan)
	}
	if !syncFlags.Yes {
		return fmt.Errorf("%w: rerun with --yes to execute sync", ErrConfirmationRequired)
	}
	result, err := eng.Execute(ctx, plan, newSyncObserver(app.Out, overrides))
	if err != nil {
		return sanitizeError(err, cfg)
	}
	return printResult(app.Out, result)
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

type syncObserver struct {
	out    io.Writer
	format string
	quiet  bool
}

func newSyncObserver(out io.Writer, overrides FlagOverrides) engine.ProgressObserver {
	return &syncObserver{out: out, format: overrides.Output, quiet: overrides.Quiet}
}

func (o *syncObserver) OnEvent(_ context.Context, event engine.Event) {
	if o.quiet || o.out == nil {
		return
	}
	if o.format == "json" {
		_, _ = fmt.Fprintf(o.out, "{\"event\":%q}\n", event.Name)
		return
	}
	_, _ = fmt.Fprintf(o.out, "%s\n", event.Name)
}

func printPlan(out io.Writer, plan *models.SyncPlan) error {
	_, err := fmt.Fprintf(out, "plan database=%s engine=%s tables=%d dry_run=%t\n",
		plan.Database, plan.Engine, len(plan.Tables), plan.DryRun)
	return err
}

func printResult(out io.Writer, result *models.SyncResult) error {
	_, err := fmt.Fprintf(out, "synced database=%s tables=%d rows=%d bytes=%d\n",
		result.Database, result.TablesCopied, result.RowsCopied, result.BytesCopied)
	return err
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

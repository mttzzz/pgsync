package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/mttzzz/pgsync/internal/engine/pgtools"
	"github.com/mttzzz/pgsync/internal/runner"
)

func newDoctorCommand(app App, globals *globalFlags) *cobra.Command {
	return &cobra.Command{Use: "doctor", Short: "Run local diagnostics", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		info := inspectPgtools(cmd.Context(), globals.UseSystemPgtools)
		if globals.Output == "json" {
			return json.NewEncoder(app.Out).Encode(map[string]string{
				"event":            "doctor.done",
				"message":          "ok",
				"pgtools_mode":     info.Mode,
				"pgtools_platform": info.Platform,
				"pg_dump":          info.PgDump,
				"pg_restore":       info.PgRestore,
				"pgtools_version":  info.Version,
				"pgtools_cache":    info.CacheRoot,
				"pgtools_hash":     info.Hash,
				"pgtools_error":    info.Error,
			})
		}
		_, _ = fmt.Fprintln(app.Out, "ok")
		_, _ = fmt.Fprintf(app.Out, "pgtools mode: %s\n", info.Mode)
		if info.Error != "" {
			_, _ = fmt.Fprintf(app.Out, "pgtools error: %s\n", info.Error)
			return nil
		}
		_, _ = fmt.Fprintf(app.Out, "pg_dump: %s\n", info.PgDump)
		_, _ = fmt.Fprintf(app.Out, "pg_restore: %s\n", info.PgRestore)
		if info.Version != "" {
			_, _ = fmt.Fprintf(app.Out, "pgtools version: %s\n", info.Version)
		}
		if info.CacheRoot != "" {
			_, _ = fmt.Fprintf(app.Out, "pgtools cache: %s\n", info.CacheRoot)
			_, _ = fmt.Fprintf(app.Out, "pgtools hash: %s\n", info.Hash)
		}
		return nil
	}}
}

type pgtoolsDoctorInfo struct {
	Mode      string `json:"mode"`
	Platform  string `json:"platform"`
	PgDump    string `json:"pg_dump,omitempty"`
	PgRestore string `json:"pg_restore,omitempty"`
	Version   string `json:"version,omitempty"`
	CacheRoot string `json:"cache_root,omitempty"`
	Hash      string `json:"hash,omitempty"`
	Error     string `json:"error,omitempty"`
}

func inspectPgtools(ctx context.Context, useSystem bool) pgtoolsDoctorInfo {
	loc := pgtools.NewLocator(pgtools.LocatorOptions{UseSystem: useSystem})
	info := pgtoolsDoctorInfo{Mode: loc.Mode(), Platform: pgtools.EmbeddedPlatform()}
	paths, err := loc.Paths(ctx)
	if err != nil {
		info.Error = err.Error()
		return info
	}
	info.PgDump = paths.PgDump
	info.PgRestore = paths.PgRestore
	info.CacheRoot = paths.Root
	info.Hash = paths.Hash
	info.Platform = paths.Platform
	if stdout, _, err := runner.NewExec().Run(ctx, paths.PgDump, []string{"--version"}, nil); err == nil {
		info.Version = strings.TrimSpace(string(stdout))
	}
	return info
}

func newListCommand(app App, globals *globalFlags) *cobra.Command {
	return &cobra.Command{Use: "list", Short: "List configured databases", Args: cobra.NoArgs, RunE: func(_ *cobra.Command, _ []string) error {
		return writeSimpleEvent(app, globals.Output, "list.empty", "No databases loaded in offline mode")
	}}
}

func newStatusCommand(app App, globals *globalFlags) *cobra.Command {
	return &cobra.Command{Use: "status", Short: "Show connection status", Args: cobra.NoArgs, RunE: func(_ *cobra.Command, _ []string) error {
		return writeSimpleEvent(app, globals.Output, "status.unknown", "Status requires configured connections")
	}}
}

func newTextCommand(app App, globals *globalFlags) *cobra.Command {
	return &cobra.Command{Use: "text", Short: "Run text-mode interactive flow", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		return runTUI(cmd.Context(), app, TUIModeConfigEdit, globals.ConfigPath)
	}}
}

func writeSimpleEvent(app App, output, event, message string) error {
	if output == "json" {
		return json.NewEncoder(app.Out).Encode(map[string]string{"event": event, "message": message})
	}
	_, err := fmt.Fprintln(app.Out, message)
	return err
}

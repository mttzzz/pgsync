// Package main provides the pgsync entrypoint.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/mttzzz/pgsync/internal/cli"
	"github.com/mttzzz/pgsync/internal/config"
	"github.com/mttzzz/pgsync/internal/engine"
	"github.com/mttzzz/pgsync/internal/engine/native"
	"github.com/mttzzz/pgsync/internal/tui"
)

func main() {
	baseLogger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	cmd := cli.NewRootCommand(cli.App{
		EngineFactory: func(logger *slog.Logger) (engine.Engine, error) {
			if logger == nil {
				logger = baseLogger
			}
			return native.NewDefault(logger)
		},
		TUIRunner: cli.TUIRunnerFunc(func(ctx context.Context, mode cli.TUIMode) error {
			program := tea.NewProgram(tui.NewApp(config.Defaults()))
			_, err := program.Run()
			return err
		}),
	})
	if err := cmd.Execute(); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(cli.ExitCode(err))
	}
}

// Package main provides the pgsync entrypoint.
package main

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/mttzzz/pgsync/internal/cli"
	"github.com/mttzzz/pgsync/internal/engine"
	"github.com/mttzzz/pgsync/internal/engine/native"
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
		TUIRunner: productionTUIRunner{},
	})
	if err := cmd.Execute(); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(cli.ExitCode(err))
	}
}

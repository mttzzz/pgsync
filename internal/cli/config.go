package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/BurntSushi/toml"
	"github.com/spf13/cobra"

	"github.com/mttzzz/pgsync/internal/config"
)

type fileConfigStore struct {
	env map[string]string
}

func (s fileConfigStore) Load(path string) (config.Config, error)   { return config.Load(path) }
func (s fileConfigStore) Save(path string, cfg config.Config) error { return config.Save(path, cfg) }
func (s fileConfigStore) DefaultPath() (string, error)              { return config.DefaultPath(s.env) }
func (s fileConfigStore) Remove(path string) error                  { return os.Remove(path) }

func newConfigCommand(app App, globals *globalFlags) *cobra.Command {
	store := fileConfigStore{env: envMap(os.Environ())}
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage pgsync configuration",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runTUI(cmd.Context(), app, TUIModeConfigEdit)
		},
	}
	cmd.AddCommand(newConfigPathCommand(app, globals, store), newConfigShowCommand(app, globals, store), newConfigResetCommand(app, globals, store))
	return cmd
}

func newConfigPathCommand(app App, globals *globalFlags, store ConfigStore) *cobra.Command {
	return &cobra.Command{Use: "path", Short: "Print config path", Args: cobra.NoArgs, RunE: func(_ *cobra.Command, _ []string) error {
		path, err := configPath(globals, store)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(app.Out, path)
		return err
	}}
}

func newConfigShowCommand(app App, globals *globalFlags, store ConfigStore) *cobra.Command {
	return &cobra.Command{Use: "show", Short: "Show redacted config", Args: cobra.NoArgs, RunE: func(_ *cobra.Command, _ []string) error {
		path, err := configPath(globals, store)
		if err != nil {
			return err
		}
		cfg, err := store.Load(path)
		if err != nil {
			return err
		}
		return writeRedactedConfig(app.Out, globals.Output, config.Redacted(cfg))
	}}
}

func newConfigResetCommand(app App, globals *globalFlags, store ConfigStore) *cobra.Command {
	return &cobra.Command{Use: "reset", Short: "Reset config through wizard", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		path, err := configPath(globals, store)
		if err == nil {
			_ = store.Remove(path)
		}
		return runTUI(cmd.Context(), app, TUIModeConfigReset)
	}}
}

func configPath(globals *globalFlags, store ConfigStore) (string, error) {
	if globals.ConfigPath != "" {
		return globals.ConfigPath, nil
	}
	return store.DefaultPath()
}

func writeRedactedConfig(w io.Writer, output string, cfg config.Config) error {
	if output == "json" {
		return json.NewEncoder(w).Encode(map[string]any{"event": "config.show", "config": cfg})
	}
	return toml.NewEncoder(w).Encode(cfg)
}

func runTUI(ctx context.Context, app App, mode TUIMode) error {
	if app.TUIRunner == nil {
		return fmt.Errorf("tui: %w", ErrNotImplemented)
	}
	return app.TUIRunner.Run(ctx, mode)
}

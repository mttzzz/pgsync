package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/mttzzz/pgsync/internal/cli"
	"github.com/mttzzz/pgsync/internal/config"
	"github.com/mttzzz/pgsync/internal/engine/native"
	"github.com/mttzzz/pgsync/internal/models"
	"github.com/mttzzz/pgsync/internal/pgdb"
	"github.com/mttzzz/pgsync/internal/pgschema"
	"github.com/mttzzz/pgsync/internal/tui"
	"github.com/mttzzz/pgsync/internal/tui/screens"
)

type productionTUIRunner struct{}

func (productionTUIRunner) Run(ctx context.Context, mode cli.TUIMode) error {
	return runInteractiveTUI(ctx, mode, "")
}

func (productionTUIRunner) RunWithConfig(ctx context.Context, mode cli.TUIMode, configPath string) error {
	return runInteractiveTUI(ctx, mode, configPath)
}

func runInteractiveTUI(ctx context.Context, mode cli.TUIMode, configPath string) error {
	cfg, path, loadErr := loadTUIConfig(configPath)
	if path == "" {
		return loadErr
	}
	if mode == cli.TUIModeConfigReset {
		cfg = config.Defaults()
	}

	validateErr := config.Validate(cfg)
	needsConfig := mode == cli.TUIModeConfigEdit || mode == cli.TUIModeConfigReset || loadErr != nil || validateErr != nil
	if needsConfig {
		if loadErr != nil && !errors.Is(loadErr, fs.ErrNotExist) {
			_, _ = fmt.Fprintf(os.Stderr, "Не удалось загрузить конфиг, открою мастер настройки: %v\n", loadErr)
		}

		editorMode := screens.EditMode
		if mode == cli.TUIModeConfigReset {
			editorMode = screens.ResetMode
		} else if mode == cli.TUIModeApp && (loadErr != nil || validateErr != nil) {
			editorMode = screens.WizardMode
		}

		form := screens.BuildEditableConfigForm(&cfg, editorMode)
		if err := form.RunWithContext(ctx); err != nil {
			return err
		}
		if err := config.Validate(cfg); err != nil {
			return fmt.Errorf("config is incomplete: %w", err)
		}
		if err := config.Save(path, cfg); err != nil {
			return fmt.Errorf("save config: %w", err)
		}
		_, _ = fmt.Fprintf(os.Stdout, "Настройки сохранены: %s\n", path)

		if mode != cli.TUIModeApp {
			return nil
		}
	}

	services := productionTUIServices(cfg)
	for {
		program := tea.NewProgram(tui.NewAppWithServices(cfg, services), tea.WithAltScreen())
		model, err := program.Run()
		if err != nil {
			return err
		}

		app, ok := model.(tui.App)
		if !ok {
			return nil
		}
		state := app.State()
		if state.Current != screens.ConfigEditorID || state.Quit {
			return nil
		}

		form := screens.BuildEditableConfigForm(&cfg, screens.EditMode)
		if err := form.RunWithContext(ctx); err != nil {
			return err
		}
		if err := config.Validate(cfg); err != nil {
			return fmt.Errorf("config is incomplete: %w", err)
		}
		if err := config.Save(path, cfg); err != nil {
			return fmt.Errorf("save config: %w", err)
		}
		services = productionTUIServices(cfg)
	}
}

func productionTUIServices(cfg config.Config) tui.Services {
	eng, err := native.NewDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})))
	services := tui.Services{Catalog: postgresCatalogService{cfg: cfg, connector: pgdb.NewConnector()}}
	if err == nil {
		services.Planner = eng
		services.Executor = eng
	}
	return services
}

type postgresCatalogService struct {
	cfg       config.Config
	connector pgdb.Connector
}

func (s postgresCatalogService) ListDatabases(ctx context.Context) ([]models.Database, error) {
	var lastErr error
	for _, database := range maintenanceDatabaseCandidates(s.cfg) {
		conn, err := s.connector.Connect(ctx, pgdb.EndpointFromConfig(s.cfg.Remote, database))
		if err != nil {
			lastErr = err
			continue
		}
		databases, listErr := pgschema.NewService(conn).ListDatabases(ctx)
		closeErr := conn.Close(context.WithoutCancel(ctx))
		if listErr != nil {
			lastErr = listErr
			continue
		}
		if closeErr != nil {
			lastErr = closeErr
			continue
		}
		return s.withTableCounts(ctx, databases), nil
	}
	return nil, lastErr
}

func (s postgresCatalogService) withTableCounts(ctx context.Context, databases []models.Database) []models.Database {
	for i := range databases {
		tables, err := s.ListTables(ctx, databases[i].Name)
		if err == nil {
			databases[i].TableCount = len(tables)
		}
	}
	return databases
}

func maintenanceDatabaseCandidates(cfg config.Config) []string {
	seen := map[string]bool{}
	candidates := make([]string, 0, 4)
	for _, database := range []string{cfg.Remote.Database, cfg.Runtime.DefaultDatabase, "defaultdb", ""} {
		database = strings.TrimSpace(database)
		if seen[database] {
			continue
		}
		seen[database] = true
		candidates = append(candidates, database)
	}
	return candidates
}

func (s postgresCatalogService) ListTables(ctx context.Context, database string) ([]models.Table, error) {
	conn, err := s.connector.Connect(ctx, pgdb.EndpointFromConfig(s.cfg.Remote, database))
	if err != nil {
		return nil, err
	}
	defer func() { _ = conn.Close(context.WithoutCancel(ctx)) }()
	return pgschema.NewService(conn).ListTables(ctx)
}

func loadTUIConfig(configPath string) (config.Config, string, error) {
	env := tuiEnvMap(os.Environ())
	path := strings.TrimSpace(configPath)
	if path == "" {
		var err error
		path, err = config.DefaultPath(env)
		if err != nil {
			return config.Config{}, "", fmt.Errorf("resolve config path: %w", err)
		}
	}

	cfg := config.Defaults()
	loaded, err := config.Load(path)
	if err == nil {
		cfg = overlayTUIConfig(cfg, loaded)
	}
	withEnv, envErr := config.ApplyEnv(cfg, env)
	if envErr != nil {
		return cfg, path, fmt.Errorf("apply env: %w", envErr)
	}
	cfg = withEnv
	return cfg, path, err
}

func overlayTUIConfig(base config.Config, override config.Config) config.Config {
	base.Remote = overlayTUIConnection(base.Remote, override.Remote)
	base.Local = overlayTUIConnection(base.Local, override.Local)
	base.Runtime = overlayTUIRuntime(base.Runtime, override.Runtime)
	base.Logging = overlayTUILogging(base.Logging, override.Logging)
	return base
}

func overlayTUIConnection(base config.Connection, override config.Connection) config.Connection {
	if override.Host != "" {
		base.Host = override.Host
	}
	if override.Port != 0 {
		base.Port = override.Port
	}
	if override.User != "" {
		base.User = override.User
	}
	if override.Password != "" {
		base.Password = override.Password
	}
	if override.Database != "" {
		base.Database = override.Database
	}
	if override.SSLMode != "" {
		base.SSLMode = override.SSLMode
	}
	if override.ProxyURL != "" {
		base.ProxyURL = override.ProxyURL
	}
	return base
}

func overlayTUIRuntime(base config.Runtime, override config.Runtime) config.Runtime {
	if override.Threads != 0 {
		base.Threads = override.Threads
	}
	if override.Engine != "" {
		base.Engine = override.Engine
	}
	if override.UseSystemPgtools {
		base.UseSystemPgtools = true
	}
	if override.DefaultDatabase != "" {
		base.DefaultDatabase = override.DefaultDatabase
	}
	if override.ConcurrentIndexes {
		base.ConcurrentIndexes = true
	}
	return base
}

func overlayTUILogging(base config.Logging, override config.Logging) config.Logging {
	if override.Level != "" {
		base.Level = override.Level
	}
	if override.Format != "" {
		base.Format = override.Format
	}
	return base
}

func tuiEnvMap(entries []string) map[string]string {
	env := make(map[string]string, len(entries))
	for _, entry := range entries {
		key, value, ok := strings.Cut(entry, "=")
		if ok {
			env[key] = value
		}
	}
	return env
}

package cli

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/spf13/cobra"

	"github.com/mttzzz/pgsync/internal/config"
	"github.com/mttzzz/pgsync/internal/engine"
	"github.com/mttzzz/pgsync/internal/engine/native"
	"github.com/mttzzz/pgsync/internal/version"
)

// ErrNotImplemented marks commands that are intentionally unavailable in Phase 2.
var ErrNotImplemented = errors.New("not implemented in Phase 2")

// ErrConfirmationRequired marks sync executions that need an explicit --yes.
var ErrConfirmationRequired = errors.New("confirmation required")

// App contains injectable CLI dependencies.
type App struct {
	EngineFactory func(*slog.Logger) (engine.Engine, error)
	TUIRunner     TUIRunner
	Out           io.Writer
	Err           io.Writer
	In            io.Reader
}

type globalFlags struct {
	ConfigPath       string
	Threads          int
	Engine           string
	UseSystemPgtools bool
	Output           string
	Quiet            bool
	Verbose          bool
	NoColor          bool
	Remote           config.Connection
	Local            config.Connection
}

// NewRootCommand builds the root pgsync command.
func NewRootCommand(app App) *cobra.Command {
	app = normalizeApp(app)
	flags := &globalFlags{Output: "text"}
	cmd := &cobra.Command{
		Use:           "pgsync",
		Short:         "PostgreSQL database sync",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if app.TUIRunner != nil {
				return runTUI(cmd.Context(), app, TUIModeApp)
			}
			return cmd.Help()
		},
	}
	cmd.SetOut(app.Out)
	cmd.SetErr(app.Err)
	cmd.SetIn(app.In)
	addGlobalFlags(cmd, flags)
	cmd.AddCommand(
		newVersionCommand(app),
		newSyncCommand(app, flags),
		newConfigCommand(app, flags),
		newTUICommand(app),
		newDoctorCommand(app, flags),
		newListCommand(app, flags),
		newStatusCommand(app, flags),
		newTextCommand(app),
	)
	cmd.AddCommand(newUnimplementedCommand("upgrade"))
	return cmd
}

// ExitCode maps command errors to process exit codes.
func ExitCode(err error) int {
	if err == nil {
		return 0
	}
	if errors.Is(err, ErrNotImplemented) {
		return 2
	}
	return 1
}

func normalizeApp(app App) App {
	if app.Out == nil {
		app.Out = os.Stdout
	}
	if app.Err == nil {
		app.Err = os.Stderr
	}
	if app.In == nil {
		app.In = os.Stdin
	}
	if app.EngineFactory == nil {
		app.EngineFactory = func(logger *slog.Logger) (engine.Engine, error) {
			return native.NewDefault(logger)
		}
	}
	return app
}

func addGlobalFlags(cmd *cobra.Command, flags *globalFlags) {
	cmd.PersistentFlags().StringVar(&flags.ConfigPath, "config", "", "path to config TOML")
	cmd.PersistentFlags().IntVar(&flags.Threads, "threads", 0, "copy worker count")
	cmd.PersistentFlags().StringVar(&flags.Engine, "engine", "", "engine mode: auto, native, external")
	cmd.PersistentFlags().BoolVar(&flags.UseSystemPgtools, "use-system-pgtools", false, "use pg_dump/pg_restore from PATH")
	cmd.PersistentFlags().StringVar(&flags.Output, "output", "text", "output format: text or json")
	cmd.PersistentFlags().BoolVar(&flags.Quiet, "quiet", false, "suppress progress output")
	cmd.PersistentFlags().BoolVar(&flags.Verbose, "verbose", false, "enable debug logging")
	cmd.PersistentFlags().BoolVar(&flags.NoColor, "no-color", false, "disable color output")
	addConnectionFlags(cmd, flags)
}

func addConnectionFlags(cmd *cobra.Command, flags *globalFlags) {
	cmd.PersistentFlags().StringVar(&flags.Remote.Host, "remote-host", "", "remote PostgreSQL host")
	cmd.PersistentFlags().IntVar(&flags.Remote.Port, "remote-port", 0, "remote PostgreSQL port")
	cmd.PersistentFlags().StringVar(&flags.Remote.User, "remote-user", "", "remote PostgreSQL user")
	cmd.PersistentFlags().StringVar(&flags.Remote.Password, "remote-password", "", "remote PostgreSQL password")
	cmd.PersistentFlags().StringVar(&flags.Remote.Database, "remote-database", "", "remote default database")
	cmd.PersistentFlags().StringVar(&flags.Remote.SSLMode, "remote-ssl-mode", "", "remote sslmode")
	cmd.PersistentFlags().StringVar(&flags.Remote.ProxyURL, "remote-proxy-url", "", "remote proxy URL")
	cmd.PersistentFlags().StringVar(&flags.Local.Host, "local-host", "", "local PostgreSQL host")
	cmd.PersistentFlags().IntVar(&flags.Local.Port, "local-port", 0, "local PostgreSQL port")
	cmd.PersistentFlags().StringVar(&flags.Local.User, "local-user", "", "local PostgreSQL user")
	cmd.PersistentFlags().StringVar(&flags.Local.Password, "local-password", "", "local PostgreSQL password")
	cmd.PersistentFlags().StringVar(&flags.Local.SSLMode, "local-ssl-mode", "", "local sslmode")
}

func newVersionCommand(app App) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			_, err := fmt.Fprintln(app.Out, version.String())
			return err
		},
	}
}

func newUnimplementedCommand(name string) *cobra.Command {
	return &cobra.Command{
		Use:   name,
		Short: "Not implemented in Phase 2",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return fmt.Errorf("%s: %w", cmd.Name(), ErrNotImplemented)
		},
	}
}

func (f *globalFlags) overrides(cmd *cobra.Command) FlagOverrides {
	flags := FlagOverrides{
		ConfigPath: f.ConfigPath,
		Threads:    f.Threads,
		Engine:     f.Engine,
		Output:     f.Output,
		Quiet:      f.Quiet,
		Verbose:    f.Verbose,
		NoColor:    f.NoColor,
		Remote:     f.Remote,
		Local:      f.Local,
	}
	if cmd.Flags().Lookup("use-system-pgtools").Changed {
		flags.UseSystemPgtools = &f.UseSystemPgtools
	}
	return flags
}

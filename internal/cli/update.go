package cli

import (
	"bufio"
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/mttzzz/pgsync/internal/updater"
	"github.com/mttzzz/pgsync/internal/version"
)

type updateFlags struct {
	CheckOnly bool
	Force     bool
}

func newUpdateCommand(app App) *cobra.Command {
	flags := &updateFlags{}
	cmd := &cobra.Command{
		Use:     "update",
		Aliases: []string{"upgrade"},
		Short:   "Check for updates and install the latest pgsync release",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runUpdate(cmd.Context(), app, *flags)
		},
	}
	cmd.Flags().BoolVar(&flags.CheckOnly, "check-only", false, "only check for updates without installing")
	cmd.Flags().BoolVar(&flags.Force, "force", false, "skip confirmation prompt")
	return cmd
}

func runUpdate(ctx context.Context, app App, flags updateFlags) error {
	if app.Updater == nil {
		client := updater.NewClient()
		app.Updater = client
	}
	_, _ = fmt.Fprintln(app.Out, "Checking for updates...")
	info, err := app.Updater.Check(ctx, version.Version)
	if err != nil {
		return fmt.Errorf("check updates: %w", err)
	}
	if !info.Available {
		_, _ = fmt.Fprintf(app.Out, "✅ You are already using the latest version (%s)\n", info.CurrentVersion)
		return nil
	}
	_, _ = fmt.Fprintln(app.Out, "")
	_, _ = fmt.Fprintln(app.Out, "🎉 New version available!")
	_, _ = fmt.Fprintf(app.Out, "   Current: %s\n", info.CurrentVersion)
	_, _ = fmt.Fprintf(app.Out, "   Latest:  %s\n", info.LatestVersion)
	_, _ = fmt.Fprintf(app.Out, "   Asset:   %s (%s)\n", info.AssetName, formatUpdateBytes(info.AssetSize))
	if info.ReleaseURL != "" {
		_, _ = fmt.Fprintf(app.Out, "   Release: %s\n", info.ReleaseURL)
	}
	if flags.CheckOnly {
		return nil
	}
	if !flags.Force {
		ok, err := confirmUpdate(app, "Download and install "+info.LatestVersion+"?")
		if err != nil {
			return err
		}
		if !ok {
			_, _ = fmt.Fprintln(app.Out, "Update cancelled")
			return nil
		}
	}
	_, _ = fmt.Fprintln(app.Out, "Downloading and installing update...")
	result, err := app.Updater.Install(ctx, info)
	if err != nil {
		return fmt.Errorf("install update: %w", err)
	}
	_, _ = fmt.Fprintln(app.Out, "✅ Update completed successfully!")
	_, _ = fmt.Fprintf(app.Out, "   Updated from %s to %s\n", result.PreviousVersion, result.NewVersion)
	_, _ = fmt.Fprintf(app.Out, "   Path: %s\n", result.Path)
	_, _ = fmt.Fprintf(app.Out, "   Duration: %s\n", result.Duration)
	return nil
}

func confirmUpdate(app App, prompt string) (bool, error) {
	_, _ = fmt.Fprintf(app.Out, "%s [y/N] ", prompt)
	line, err := bufio.NewReader(app.In).ReadString('\n')
	if err != nil && strings.TrimSpace(line) == "" {
		return false, err
	}
	line = strings.ToLower(strings.TrimSpace(line))
	return line == "y" || line == "yes", nil
}

func formatUpdateBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	divisor, exp := int64(unit), 0
	for value := bytes / unit; value >= unit; value /= unit {
		divisor *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(divisor), "KMGTPE"[exp])
}

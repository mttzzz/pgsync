package cli

import (
	"github.com/spf13/cobra"
)

func newTUICommand(app App) *cobra.Command {
	return &cobra.Command{
		Use:   "tui",
		Short: "Launch full-screen TUI",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runTUI(cmd.Context(), app, TUIModeApp)
		},
	}
}

package cli

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

func newDoctorCommand(app App, globals *globalFlags) *cobra.Command {
	return &cobra.Command{Use: "doctor", Short: "Run local diagnostics", Args: cobra.NoArgs, RunE: func(_ *cobra.Command, _ []string) error {
		return writeSimpleEvent(app, globals.Output, "doctor.done", "ok")
	}}
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

func newTextCommand(app App) *cobra.Command {
	return &cobra.Command{Use: "text", Short: "Run text-mode interactive flow", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		return runTUI(cmd.Context(), app, TUIModeConfigEdit)
	}}
}

func writeSimpleEvent(app App, output, event, message string) error {
	if output == "json" {
		return json.NewEncoder(app.Out).Encode(map[string]string{"event": event, "message": message})
	}
	_, err := fmt.Fprintln(app.Out, message)
	return err
}

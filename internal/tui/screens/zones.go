package screens

import "fmt"

const (
	ActionTables    = "tables"
	ActionSelectAll = "select-all"
	ActionClear     = "clear"
	ActionReload    = "reload"
	ActionConfirm   = "confirm"
	ActionSettings  = "settings"
	ActionBack      = "back"
	ActionCancel    = "cancel"
	ActionStart     = "start"
	ActionQuit      = "quit"
	ActionRunAgain  = "run-again"
)

// DatabaseRowZone returns the BubbleZone id for a database row.
func DatabaseRowZone(index int) string { return fmt.Sprintf("db-row:%d", index) }

// TableRowZone returns the BubbleZone id for a table row.
func TableRowZone(index int) string { return fmt.Sprintf("table-row:%d", index) }

// ActionZone returns the BubbleZone id for a named action.
func ActionZone(action string) string { return "action:" + action }

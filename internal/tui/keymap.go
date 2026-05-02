package tui

import tea "github.com/charmbracelet/bubbletea"

// KeyAction identifies a global key action.
type KeyAction string

const (
	// KeyNone means no global key action.
	KeyNone KeyAction = "none"
	// KeyOpenConfig opens configuration.
	KeyOpenConfig KeyAction = "open-config"
	// KeyBack navigates back.
	KeyBack KeyAction = "back"
	// KeyQuit quits the application.
	KeyQuit KeyAction = "quit"
	// KeyConfirm confirms the focused action.
	KeyConfirm KeyAction = "confirm"
	// KeyTogglePause pauses or resumes sync.
	KeyTogglePause KeyAction = "toggle-pause"
)

// GlobalKeyAction maps a Bubble Tea key message to a global action.
func GlobalKeyAction(msg tea.KeyMsg) KeyAction {
	switch msg.String() {
	case "s":
		return KeyOpenConfig
	case "esc":
		return KeyBack
	case "q", "ctrl+c":
		return KeyQuit
	case "enter":
		return KeyConfirm
	case " ":
		return KeyTogglePause
	default:
		return KeyNone
	}
}

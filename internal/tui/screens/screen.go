// Package screens contains pure TUI screen models.
package screens

import (
	tea "github.com/charmbracelet/bubbletea"
	zone "github.com/lrstanley/bubblezone"
)

// ID identifies a screen in the app state machine.
type ID string

const (
	// SettingsCheckID identifies settings validation.
	SettingsCheckID ID = "settings-check"
	// MainMenuID identifies the main menu.
	MainMenuID ID = "main-menu"
	// ConfigEditorID identifies the config editor.
	ConfigEditorID ID = "config-editor"
	// DatabaseListID identifies database selection.
	DatabaseListID ID = "database-list"
	// ConfirmPlanID identifies plan confirmation.
	ConfirmPlanID ID = "confirm-plan"
	// ProgressID identifies sync progress.
	ProgressID ID = "progress"
	// ResultID identifies sync results.
	ResultID ID = "result"
)

// Screen is the common contract for testable Bubble Tea screens.
type Screen interface {
	Init() tea.Cmd
	Update(tea.Msg) (Screen, tea.Cmd)
	View() string
	ID() ID
	Title() string
	Help() string
}

// StaticScreen is a simple immutable screen useful for routing and tests.
type StaticScreen struct {
	ScreenID ID
	Heading  string
	Body     string
	Hint     string
}

// Init returns no command.
func (s StaticScreen) Init() tea.Cmd { return nil }

// Update returns the same screen.
func (s StaticScreen) Update(tea.Msg) (Screen, tea.Cmd) { return s, nil }

// View renders the body.
func (s StaticScreen) View() string { return zone.Scan(s.Body) }

// ID returns the screen ID.
func (s StaticScreen) ID() ID { return s.ScreenID }

// Title returns the screen title.
func (s StaticScreen) Title() string { return s.Heading }

// Help returns help text.
func (s StaticScreen) Help() string { return s.Hint }

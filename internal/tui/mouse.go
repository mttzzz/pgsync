package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	zone "github.com/lrstanley/bubblezone"

	"github.com/mttzzz/pgsync/internal/tui/screens"
)

func (a App) onMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if msg.Action != tea.MouseActionRelease || msg.Button != tea.MouseButtonLeft {
		return a, nil
	}
	switch a.state.Current {
	case screens.DatabaseListID:
		return a.onDatabaseMouse(msg)
	case screens.ConfirmPlanID:
		return a.onConfirmMouse(msg)
	case screens.ResultID:
		return a.onResultMouse(msg)
	default:
		return a, nil
	}
}

//nolint:gocognit,gocyclo // Mirrors keyboard database actions for each clickable zone explicitly.
func (a App) onDatabaseMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	for index, db := range a.state.Databases {
		if zone.Get(screens.DatabaseRowZone(index)).InBounds(msg) {
			a.state.DatabaseIndex = index
			if a.state.SelectedDatabases == nil {
				a.state.SelectedDatabases = map[string]bool{}
			}
			a.state.SelectedDatabases[db.Name] = !a.state.SelectedDatabases[db.Name]
			if !a.state.SelectedDatabases[db.Name] {
				delete(a.state.SelectedDatabases, db.Name)
			}
			return a, nil
		}
	}
	if zone.Get(screens.ActionZone(screens.ActionSelectAll)).InBounds(msg) {
		if a.state.SelectedDatabases == nil {
			a.state.SelectedDatabases = map[string]bool{}
		}
		for _, db := range a.state.Databases {
			a.state.SelectedDatabases[db.Name] = true
		}
		return a, nil
	}
	if zone.Get(screens.ActionZone(screens.ActionClear)).InBounds(msg) {
		a.state.SelectedDatabases = map[string]bool{}
		return a, nil
	}
	if zone.Get(screens.ActionZone(screens.ActionReload)).InBounds(msg) {
		a.state.Status = "Loading remote databases..."
		a.state.Err = nil
		if a.services.Catalog != nil {
			return a, loadDatabasesCmd(a.services.Catalog)
		}
		return a, nil
	}
	if zone.Get(screens.ActionZone(screens.ActionConfirm)).InBounds(msg) && len(a.state.SelectedDatabases) > 0 {
		a.state.Current = screens.ConfirmPlanID
		return a, nil
	}
	return a, nil
}

func (a App) onConfirmMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if zone.Get(screens.ActionZone(screens.ActionCancel)).InBounds(msg) || zone.Get(screens.ActionZone(screens.ActionBack)).InBounds(msg) {
		a.state.Current = screens.DatabaseListID
		return a, nil
	}
	if zone.Get(screens.ActionZone(screens.ActionStart)).InBounds(msg) || zone.Get(screens.ActionZone(screens.ActionConfirm)).InBounds(msg) {
		return a.onConfirmKey(tea.KeyMsg{Type: tea.KeyEnter})
	}
	return a, nil
}

func (a App) onResultMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if zone.Get(screens.ActionZone(screens.ActionBack)).InBounds(msg) {
		a.state.Current = screens.DatabaseListID
		return a, nil
	}
	if zone.Get(screens.ActionZone(screens.ActionRunAgain)).InBounds(msg) {
		a.state.Current = screens.ConfirmPlanID
		return a, nil
	}
	if zone.Get(screens.ActionZone(screens.ActionQuit)).InBounds(msg) {
		a.state.Quit = true
		return a, tea.Quit
	}
	return a, nil
}

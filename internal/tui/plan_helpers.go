package tui

import "github.com/mttzzz/pgsync/internal/models"

func (a App) currentPlanTables() []models.Table {
	if len(a.state.Tables) == 0 {
		return nil
	}
	selectedTables := make([]models.Table, 0, len(a.state.Tables))
	for _, table := range a.state.Tables {
		if a.state.SelectedTables == nil || a.state.SelectedTables[tableKey(table)] {
			selectedTables = append(selectedTables, table)
		}
	}
	return selectedTables
}

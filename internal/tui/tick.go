package tui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

const progressTickInterval = 100 * time.Millisecond

type progressTickMsg struct{ Time time.Time }

func progressTickCmd() tea.Cmd {
	return tea.Tick(progressTickInterval, func(t time.Time) tea.Msg {
		return progressTickMsg{Time: t}
	})
}

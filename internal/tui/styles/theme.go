// Package styles defines reusable TUI styles.
package styles

import "github.com/charmbracelet/lipgloss"

// Theme contains resolved TUI styles.
type Theme struct {
	NoColor bool
	Width   int
	Title   lipgloss.Style
	Error   lipgloss.Style
	Muted   lipgloss.Style
}

// NewTheme builds a theme from terminal options.
func NewTheme(noColor bool, width int) Theme {
	t := Theme{NoColor: noColor, Width: width}
	if noColor {
		t.Title = lipgloss.NewStyle()
		t.Error = lipgloss.NewStyle()
		t.Muted = lipgloss.NewStyle()
		return t
	}
	t.Title = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
	t.Error = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	t.Muted = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	return t
}

// Trim safely truncates text to terminal width.
func (t Theme) Trim(text string) string {
	if t.Width <= 0 || len([]rune(text)) <= t.Width {
		return text
	}
	runes := []rune(text)
	return string(runes[:t.Width])
}

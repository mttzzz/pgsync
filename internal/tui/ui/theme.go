package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Color palette values define the shared TUI theme.
var (
	ColorBg      = lipgloss.Color("#0B1020")
	ColorPanel   = lipgloss.Color("#111827")
	ColorBorder  = lipgloss.Color("#334155")
	ColorPrimary = lipgloss.Color("#8B5CF6")
	ColorAccent  = lipgloss.Color("#22D3EE")
	ColorSuccess = lipgloss.Color("#34D399")
	ColorWarning = lipgloss.Color("#FBBF24")
	ColorDanger  = lipgloss.Color("#FB7185")
	ColorText    = lipgloss.Color("#E5E7EB")
	ColorMuted   = lipgloss.Color("#94A3B8")
)

// Styles contains the shared pgsync visual system.
type Styles struct {
	Page        lipgloss.Style
	Header      lipgloss.Style
	HeaderTitle lipgloss.Style
	Panel       lipgloss.Style
	PanelTitle  lipgloss.Style
	Muted       lipgloss.Style
	Primary     lipgloss.Style
	Accent      lipgloss.Style
	Success     lipgloss.Style
	Warning     lipgloss.Style
	Danger      lipgloss.Style
	Row         lipgloss.Style
	SelectedRow lipgloss.Style
	Footer      lipgloss.Style
	Key         lipgloss.Style
	Button      lipgloss.Style
	HotButton   lipgloss.Style
}

// NewStyles creates a consistent high-contrast theme.
func NewStyles() Styles {
	return Styles{
		Page:        lipgloss.NewStyle().Padding(1, 2),
		Header:      lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(ColorPrimary).Padding(0, 2).Foreground(ColorText),
		HeaderTitle: lipgloss.NewStyle().Bold(true).Foreground(ColorText),
		Panel:       lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(ColorBorder).Padding(1, 2).Foreground(ColorText),
		PanelTitle:  lipgloss.NewStyle().Bold(true).Foreground(ColorText),
		Muted:       lipgloss.NewStyle().Foreground(ColorMuted),
		Primary:     lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true),
		Accent:      lipgloss.NewStyle().Foreground(ColorAccent).Bold(true),
		Success:     lipgloss.NewStyle().Foreground(ColorSuccess).Bold(true),
		Warning:     lipgloss.NewStyle().Foreground(ColorWarning).Bold(true),
		Danger:      lipgloss.NewStyle().Foreground(ColorDanger).Bold(true),
		Row:         lipgloss.NewStyle().Foreground(ColorText),
		SelectedRow: lipgloss.NewStyle().Foreground(ColorText).Background(lipgloss.Color("#312E81")).Bold(true),
		Footer:      lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(ColorBorder).Padding(0, 2).Foreground(ColorMuted),
		Key:         lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true),
		Button:      lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(ColorBorder).Padding(0, 2).Foreground(ColorText),
		HotButton:   lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(ColorPrimary).Padding(0, 2).Foreground(ColorPrimary).Bold(true),
	}
}

// Dot renders a colored status dot with a label.
func Dot(style lipgloss.Style, label string) string { return style.Render("● " + label) }

// Metric renders a compact name/value pair.
func Metric(label, value string, valueStyle lipgloss.Style) string {
	return lipgloss.NewStyle().Foreground(ColorMuted).Render(label) + " " + valueStyle.Render(value)
}

// SectionTitle renders a panel title line.
func SectionTitle(title string) string { return NewStyles().PanelTitle.Render(title) }

// HeaderLine pads a header line to the requested width when possible.
func HeaderLine(left, right string, width int) string {
	if width <= 0 {
		return left + "  " + right
	}
	gap := width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 2 {
		gap = 2
	}
	return left + strings.Repeat(" ", gap) + right
}

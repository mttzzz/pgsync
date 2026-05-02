package screens

import (
	"strings"

	"github.com/mttzzz/pgsync/internal/config"
	"github.com/mttzzz/pgsync/internal/tui/styles"
)

// Frame wraps screen body with title, status, and help.
func Frame(theme styles.Theme, title, body, help, status string) string {
	parts := []string{theme.Title.Render(theme.Trim(title)), theme.Trim(body)}
	if status != "" {
		parts = append(parts, theme.Muted.Render(theme.Trim(status)))
	}
	if help != "" {
		parts = append(parts, theme.Muted.Render(theme.Trim(help)))
	}
	return strings.Join(parts, "\n")
}

// ErrorPanel renders an error safely.
func ErrorPanel(theme styles.Theme, err error) string {
	if err == nil {
		return ""
	}
	return theme.Error.Render("Ошибка: " + RedactText(err.Error()))
}

// RedactText removes obvious secrets from defensive screen rendering.
func RedactText(text string) string {
	return config.RedactProxyURL(text)
}

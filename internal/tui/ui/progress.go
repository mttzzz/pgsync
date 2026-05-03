package ui

import (
	"strings"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/harmonica"
)

// SmoothProgress advances a displayed value toward a target with spring motion.
func SmoothProgress(current, velocity, target float64) (float64, float64) {
	spring := harmonica.NewSpring(harmonica.FPS(60), 8.0, 0.85)
	return spring.Update(current, velocity, target)
}

// ProgressBar renders a gradient progress bar with Bubbles.
func ProgressBar(width int, percent float64) string {
	if width < 12 {
		width = 12
	}
	model := progress.New(
		progress.WithGradient("#8B5CF6", "#22D3EE"),
		progress.WithoutPercentage(),
	)
	model.Width = width
	return model.ViewAs(clampProgress(percent / 100))
}

// MiniBar renders a compact fixed-cell bar for dense table sections.
func MiniBar(width int, percent float64) string {
	value := clampProgress(percent / 100)
	filled := int(value * float64(width))
	if filled > width {
		filled = width
	}
	styles := NewStyles()
	return styles.Success.Render(strings.Repeat("█", filled)) + styles.Muted.Render(strings.Repeat("░", width-filled))
}

func clampProgress(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}

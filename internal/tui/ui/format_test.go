package ui

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/stretchr/testify/assert"

	"github.com/mttzzz/pgsync/internal/config"
)

func TestFormatNumbersDurationsAndRates(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "0", FormatInt(0))
	assert.Equal(t, "-42", FormatInt(-42))
	assert.Equal(t, "999", FormatInt(999))
	assert.Equal(t, "1 234 567", FormatInt(1_234_567))
	assert.Equal(t, "-1 234", FormatInt(-1_234))
	assert.Equal(t, "12 345", FormatCount(12_345))

	assert.Equal(t, "12.3%", FormatPercent(12.34))
	assert.Equal(t, "0.0%", FormatPercent(math.NaN()))
	assert.Equal(t, "0.0%", FormatPercent(math.Inf(1)))

	assert.Equal(t, "0.0s", FormatDurationTenths(-time.Second))
	assert.Equal(t, "1.2s", FormatDurationTenths(1230*time.Millisecond))
	assert.Equal(t, "01:05.0", FormatDurationTenths(65*time.Second))
	assert.Equal(t, "01:01:01.0", FormatDurationTenths(time.Hour+time.Minute+time.Second))

	assert.Equal(t, "42 B", FormatBytes(42))
	assert.Equal(t, "1.0 KB", FormatBytes(1024))
	assert.Equal(t, "1.5 MB", FormatBytes(1536*1024))
	assert.Equal(t, "0 B/s", FormatBytesRate(0))
	assert.Equal(t, "0 B/s", FormatBytesRate(math.NaN()))
	assert.Equal(t, "1.0 KB/s", FormatBytesRate(1024))
	assert.Equal(t, "0 rows/s", FormatRowsRate(0))
	assert.Equal(t, "0 rows/s", FormatRowsRate(math.Inf(1)))
	assert.Equal(t, "1 235 rows/s", FormatRowsRate(1234.6))
}

func TestEndpointTransportAndCopyLabels(t *testing.T) {
	t.Parallel()
	conn := config.Connection{Host: "db.internal", Port: 5432, User: "alice", Database: "app", SSLMode: "require"}
	assert.Equal(t, "host db.internal:5432  user alice  database app  ssl require", EndpointLabel(conn, ""))
	assert.Equal(t, "host -", EndpointLabel(config.Connection{}, ""))
	assert.Equal(t, "host db.internal:5432  user alice  database other  ssl require", EndpointLabel(conn, "other"))

	assert.Equal(t, "DIRECT", ConnectionMode(config.Connection{}))
	assert.Equal(t, "PROXY", ConnectionMode(config.Connection{ProxyURL: "socks5://localhost:1080"}))
	assert.Equal(t, "off", ProxyLabel(config.Connection{}))
	assert.Equal(t, "socks5://u:xxxxx@localhost:1080", ProxyLabel(config.Connection{ProxyURL: "socks5://u:secret@localhost:1080"}))

	assert.Equal(t, "System pg_dump → pg_restore", CopyTechnology("external", true))
	assert.Equal(t, "Embedded pg_dump → pg_restore", CopyTechnology("external", false))
	assert.Equal(t, "Auto · system pgtools fallback", CopyTechnology("auto", true))
	assert.Equal(t, "Auto · native/embedded best available", CopyTechnology(" auto ", false))
	assert.Equal(t, "Native pgx COPY protocol", CopyTechnology("native", false))
	assert.Equal(t, "-", emptyDash(" "))
	assert.Equal(t, "value", emptyDash("value"))
}

func TestProgressAndThemeHelpers(t *testing.T) {
	t.Parallel()
	current, velocity := SmoothProgress(0, 0, 50)
	assert.False(t, math.IsNaN(current))
	assert.False(t, math.IsNaN(velocity))

	assert.NotEmpty(t, ProgressBar(4, -10))
	assert.NotEmpty(t, ProgressBar(20, 150))
	mini := MiniBar(4, 50)
	assert.NotEmpty(t, mini)
	assert.NotEmpty(t, MiniBar(4, 150))
	assert.Equal(t, 0.0, clampProgress(-1))
	assert.Equal(t, 0.5, clampProgress(0.5))
	assert.Equal(t, 1.0, clampProgress(2))

	styles := NewStyles()
	assert.NotEqual(t, lipgloss.Style{}, styles.Page)
	assert.Contains(t, Dot(styles.Success, "ok"), "ok")
	assert.Contains(t, Metric("Rows", "10", styles.Accent), "Rows")
	assert.Contains(t, SectionTitle("Summary"), "Summary")
	assert.Contains(t, HeaderLine("left", "right", 30), "left")
	assert.Contains(t, HeaderLine(strings.Repeat("x", 30), "right", 5), "right")
	assert.Equal(t, "left  right", HeaderLine("left", "right", 0))
}

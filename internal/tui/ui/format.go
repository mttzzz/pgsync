// Package ui contains reusable TUI presentation helpers.
package ui

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/mttzzz/pgsync/internal/config"
)

// FormatInt renders integers with thin group separators that remain readable in terminals.
func FormatInt(n int64) string {
	negative := n < 0
	if negative {
		n = -n
	}
	digits := strconv.FormatInt(n, 10)
	if len(digits) <= 3 {
		if negative {
			return "-" + digits
		}
		return digits
	}
	groups := make([]string, 0, (len(digits)+2)/3)
	for len(digits) > 3 {
		groups = append(groups, digits[len(digits)-3:])
		digits = digits[:len(digits)-3]
	}
	groups = append(groups, digits)
	for left, right := 0, len(groups)-1; left < right; left, right = left+1, right-1 {
		groups[left], groups[right] = groups[right], groups[left]
	}
	formatted := strings.Join(groups, " ")
	if negative {
		return "-" + formatted
	}
	return formatted
}

// FormatCount renders regular int counters with grouping.
func FormatCount(n int) string { return FormatInt(int64(n)) }

// FormatPercent renders a percentage rounded to tenths.
func FormatPercent(percent float64) string {
	if math.IsNaN(percent) || math.IsInf(percent, 0) {
		percent = 0
	}
	return fmt.Sprintf("%.1f%%", percent)
}

// FormatDurationTenths renders a duration rounded to tenths of a second.
func FormatDurationTenths(duration time.Duration) string {
	if duration < 0 {
		duration = 0
	}
	tenths := int64(math.Round(float64(duration) / float64(100*time.Millisecond)))
	totalSeconds := tenths / 10
	fraction := tenths % 10
	minutes := totalSeconds / 60
	seconds := totalSeconds % 60
	if minutes >= 60 {
		hours := minutes / 60
		minutes %= 60
		return fmt.Sprintf("%02d:%02d:%02d.%d", hours, minutes, seconds, fraction)
	}
	if minutes > 0 {
		return fmt.Sprintf("%02d:%02d.%d", minutes, seconds, fraction)
	}
	return fmt.Sprintf("%d.%ds", seconds, fraction)
}

// FormatBytes renders a byte amount using binary units and grouped whole bytes.
func FormatBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return FormatInt(n) + " B"
	}
	divisor, exp := int64(unit), 0
	for value := n / unit; value >= unit; value /= unit {
		divisor *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(divisor), "KMGTPE"[exp])
}

// FormatBytesRate renders bytes per second rounded to tenths.
func FormatBytesRate(bytesPerSecond float64) string {
	if bytesPerSecond <= 0 || math.IsNaN(bytesPerSecond) || math.IsInf(bytesPerSecond, 0) {
		return "0 B/s"
	}
	return FormatBytes(int64(bytesPerSecond)) + "/s"
}

// FormatRowsRate renders rows per second with grouped integer precision.
func FormatRowsRate(rowsPerSecond float64) string {
	if rowsPerSecond <= 0 || math.IsNaN(rowsPerSecond) || math.IsInf(rowsPerSecond, 0) {
		return "0 rows/s"
	}
	return FormatInt(int64(math.Round(rowsPerSecond))) + " rows/s"
}

// EndpointLabel renders a safe endpoint summary without passwords.
func EndpointLabel(conn config.Connection, database string) string {
	if database == "" {
		database = conn.Database
	}
	host := emptyDash(conn.Host)
	if conn.Port > 0 {
		host = fmt.Sprintf("%s:%d", host, conn.Port)
	}
	parts := []string{"host " + host}
	if conn.User != "" || database != "" {
		parts = append(parts, "user "+emptyDash(conn.User), "database "+emptyDash(database))
	}
	if conn.SSLMode != "" {
		parts = append(parts, "ssl "+conn.SSLMode)
	}
	return strings.Join(parts, "  ")
}

// ConnectionMode returns the transport mode shown in the header.
func ConnectionMode(remote config.Connection) string {
	if strings.TrimSpace(remote.ProxyURL) != "" {
		return "PROXY"
	}
	return "DIRECT"
}

// ProxyLabel returns a redacted proxy label or off when no proxy is configured.
func ProxyLabel(remote config.Connection) string {
	if strings.TrimSpace(remote.ProxyURL) == "" {
		return "off"
	}
	return config.RedactProxyURL(remote.ProxyURL)
}

// CopyTechnology explains which copy implementation will be used.
func CopyTechnology(engine string, useSystemPgtools bool) string {
	switch strings.TrimSpace(strings.ToLower(engine)) {
	case "external":
		if useSystemPgtools {
			return "System pg_dump → pg_restore"
		}
		return "Embedded pg_dump → pg_restore"
	case "auto":
		if useSystemPgtools {
			return "Auto · system pgtools fallback"
		}
		return "Auto · native/embedded best available"
	default:
		return "Native pgx COPY protocol"
	}
}

func emptyDash(value string) string {
	if strings.TrimSpace(value) == "" {
		return "-"
	}
	return value
}

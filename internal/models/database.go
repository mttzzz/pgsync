// Package models defines project-wide data transfer structures.
package models

import "fmt"

/* Database describes a PostgreSQL database discovered by pgsync. */
type Database struct {
	Name       string
	SizeBytes  int64
	Owner      string
	TableCount int
}

/* String returns a compact human-readable database summary. */
func (d Database) String() string {
	if d.TableCount > 0 {
		return fmt.Sprintf("%s (%s, %d tables)", d.Name, FormatBytes(d.SizeBytes), d.TableCount)
	}
	return fmt.Sprintf("%s (%s)", d.Name, FormatBytes(d.SizeBytes))
}

/* FormatBytes is a small human-friendly byte formatter. */
func FormatBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	divisor, exp := int64(unit), 0
	for value := n / unit; value >= unit; value /= unit {
		divisor *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(divisor), "KMGTPE"[exp])
}

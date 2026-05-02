// Package version exposes build-time identity.
package version

import "fmt"

var (
	/* Version is the semantic version or dev for unreleased builds. */
	Version = "dev"
	/* GitCommit is the source revision embedded at build time. */
	GitCommit = "none"
	/* BuildDate is the build timestamp embedded at build time. */
	BuildDate = "unknown"
)

/* String returns the full build identity. */
func String() string {
	return fmt.Sprintf("pgsync %s (commit %s, built %s)", Version, GitCommit, BuildDate)
}

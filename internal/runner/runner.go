// Package runner defines the seam between pgsync and child processes.
package runner

import (
	"context"
	"fmt"
)

/* CommandRunner runs a child process and captures output. */
type CommandRunner interface {
	Run(ctx context.Context, name string, args []string, env []string) (stdout []byte, stderr []byte, err error)
}

/* ExitError is returned when a child process exits non-zero. */
type ExitError struct {
	Code   int
	Stderr []byte
}

/* Error returns a human-readable child-process exit description. */
func (e *ExitError) Error() string {
	return fmt.Sprintf("exit code %d", e.Code)
}

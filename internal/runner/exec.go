package runner

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
)

/* Exec is a CommandRunner backed by os/exec. */
type Exec struct{}

/* NewExec returns an os/exec-backed CommandRunner. */
func NewExec() *Exec { return &Exec{} }

/* Run executes name with args and optional env, returning stdout and stderr. */
func (e *Exec) Run(ctx context.Context, name string, args, env []string) ([]byte, []byte, error) {
	cmd := exec.CommandContext(ctx, name, args...) // #nosec G204 -- commands are selected by trusted engine/CLI wiring.
	if env != nil {
		cmd.Env = env
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return stdout.Bytes(), stderr.Bytes(), &ExitError{
				Code:   exitErr.ExitCode(),
				Stderr: stderr.Bytes(),
			}
		}
		return stdout.Bytes(), stderr.Bytes(), err
	}
	return stdout.Bytes(), stderr.Bytes(), nil
}

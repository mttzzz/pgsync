package runner_test

import (
	"context"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mttzzz/pgsync/internal/runner"
)

func TestExecRunSuccess(t *testing.T) {
	t.Parallel()
	r := runner.NewExec()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	name, args := echoCommand("hello")
	stdout, _, err := r.Run(ctx, name, args, nil)
	require.NoError(t, err)
	assert.Contains(t, string(stdout), "hello")
}

func TestExecRunWithEnv(t *testing.T) {
	t.Parallel()
	r := runner.NewExec()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	name, args := envCommand()
	stdout, _, err := r.Run(ctx, name, args, append([]string{}, "PGSYNC_RUNNER_TEST=ok"))
	require.NoError(t, err)
	assert.Contains(t, string(stdout), "ok")
}

func TestExecRunFailingExitCode(t *testing.T) {
	t.Parallel()
	r := runner.NewExec()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	name, args := exitCommand()
	_, _, err := r.Run(ctx, name, args, nil)
	require.Error(t, err)
	var exitErr *runner.ExitError
	require.ErrorAs(t, err, &exitErr)
	assert.Equal(t, 7, exitErr.Code)
	assert.Equal(t, "exit code 7", exitErr.Error())
}

func TestExecRunCancellation(t *testing.T) {
	t.Parallel()
	r := runner.NewExec()
	ctx, cancel := context.WithCancel(t.Context())

	name, args := sleepCommand()
	done := make(chan error, 1)
	go func() {
		_, _, err := r.Run(ctx, name, args, nil)
		done <- err
	}()
	time.Sleep(50 * time.Millisecond)
	cancel()
	err := <-done
	require.Error(t, err)
}

func TestExecRunStartError(t *testing.T) {
	t.Parallel()
	r := runner.NewExec()
	stdout, stderr, err := r.Run(t.Context(), "definitely-not-a-real-pgsync-test-binary", nil, nil)
	require.Error(t, err)
	var exitErr *runner.ExitError
	assert.NotErrorAs(t, err, &exitErr)
	assert.Empty(t, stdout)
	assert.Empty(t, stderr)
}

func echoCommand(s string) (string, []string) {
	if runtime.GOOS == "windows" {
		return "cmd", []string{"/c", "echo", s}
	}
	return "sh", []string{"-c", "echo " + s}
}

func envCommand() (string, []string) {
	if runtime.GOOS == "windows" {
		return "cmd", []string{"/c", "echo", "%PGSYNC_RUNNER_TEST%"}
	}
	return "sh", []string{"-c", "printf %s ${PGSYNC_RUNNER_TEST}"}
}

func exitCommand() (string, []string) {
	if runtime.GOOS == "windows" {
		return "cmd", []string{"/c", "exit", "/b", "7"}
	}
	return "sh", []string{"-c", "exit 7"}
}

func sleepCommand() (string, []string) {
	if runtime.GOOS == "windows" {
		return "cmd", []string{"/c", "ping", "-n", "11", "127.0.0.1"}
	}
	return "sh", []string{"-c", "sleep 10"}
}

package cli

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDiagnosticCommands(t *testing.T) {
	t.Parallel()
	out, _, err := executeRoot(t, appWithEngine(&fakeEngine{}), "doctor")
	require.NoError(t, err)
	assert.Contains(t, out, "ok")

	out, _, err = executeRoot(t, appWithEngine(&fakeEngine{}), "--output=json", "list")
	require.NoError(t, err)
	assert.Contains(t, out, "list.empty")

	out, _, err = executeRoot(t, appWithEngine(&fakeEngine{}), "status")
	require.NoError(t, err)
	assert.Contains(t, out, "Status requires")
}

func TestTextCommandAndRunnerFunc(t *testing.T) {
	t.Parallel()
	runner := TUIRunnerFunc(func(ctx context.Context, mode TUIMode) error {
		assert.Equal(t, TUIModeConfigEdit, mode)
		return nil
	})
	_, _, err := executeRoot(t, App{TUIRunner: runner, EngineFactory: appWithEngine(&fakeEngine{}).EngineFactory}, "text")
	require.NoError(t, err)

	bad := TUIRunnerFunc(func(ctx context.Context, mode TUIMode) error { return errors.New("no tty") })
	_, _, err = executeRoot(t, App{TUIRunner: bad, EngineFactory: appWithEngine(&fakeEngine{}).EngineFactory}, "text")
	require.Error(t, err)
}

func TestWriteSimpleEventWriterError(t *testing.T) {
	t.Parallel()
	err := writeSimpleEvent(App{Out: errWriter{}}, "text", "x", "m")
	require.Error(t, err)
	err = writeSimpleEvent(App{Out: errWriter{}}, "json", "x", "m")
	require.Error(t, err)
	var buf bytes.Buffer
	require.NoError(t, writeSimpleEvent(App{Out: &buf}, "json", "x", "m"))
	assert.Contains(t, buf.String(), "x")
}

type errWriter struct{}

func (errWriter) Write([]byte) (int, error) { return 0, errors.New("write") }

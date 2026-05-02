package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRootNoArgsPrintsHelp(t *testing.T) {
	t.Parallel()
	out, _, err := executeRoot(t, appWithEngine(&fakeEngine{}))
	require.NoError(t, err)
	assert.Contains(t, out, "PostgreSQL database sync")
	assert.Contains(t, out, "sync")
}

func TestVersionCommand(t *testing.T) {
	t.Parallel()
	fake := &fakeEngine{}
	out, errOut, err := executeRoot(t, appWithEngine(fake), "version")
	require.NoError(t, err)
	assert.Contains(t, out, "pgsync")
	assert.Empty(t, errOut)
	assert.Zero(t, fake.planCalls)
}

func TestUnimplementedCommand(t *testing.T) {
	t.Parallel()
	_, _, err := executeRoot(t, appWithEngine(&fakeEngine{}), "doctor")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNotImplemented)
	assert.Equal(t, 2, ExitCode(err))
}

func TestNormalizeAppSuppliesDefaults(t *testing.T) {
	t.Parallel()
	app := normalizeApp(App{})
	require.NotNil(t, app.Out)
	require.NotNil(t, app.Err)
	require.NotNil(t, app.In)
	eng, err := app.EngineFactory(nil)
	require.NoError(t, err)
	assert.NotNil(t, eng)
}

func executeRoot(t *testing.T, app App, args ...string) (string, string, error) {
	t.Helper()
	var out bytes.Buffer
	var errOut bytes.Buffer
	app.Out = &out
	app.Err = &errOut
	app.In = strings.NewReader("")
	cmd := NewRootCommand(app)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), errOut.String(), err
}

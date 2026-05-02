package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mttzzz/pgsync/internal/updater"
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

func TestUpdateCommandCheckOnlyAndUpgradeAlias(t *testing.T) {
	t.Parallel()
	fake := &fakeUpdater{info: updater.UpdateInfo{Available: true, CurrentVersion: "dev", LatestVersion: "v1.2.3", AssetName: "pgsync-darwin-arm64", AssetSize: 2048, ReleaseURL: "https://example/release"}}
	out, _, err := executeRoot(t, App{Updater: fake, EngineFactory: appWithEngine(&fakeEngine{}).EngineFactory}, "update", "--check-only")
	require.NoError(t, err)
	assert.Contains(t, out, "New version available")
	assert.False(t, fake.installed)

	fake = &fakeUpdater{info: updater.UpdateInfo{Available: false, CurrentVersion: "v1.2.3", LatestVersion: "v1.2.3"}}
	out, _, err = executeRoot(t, App{Updater: fake, EngineFactory: appWithEngine(&fakeEngine{}).EngineFactory}, "upgrade")
	require.NoError(t, err)
	assert.Contains(t, out, "latest version")
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

type fakeUpdater struct {
	info      updater.UpdateInfo
	result    updater.UpdateResult
	err       error
	installed bool
}

func (f *fakeUpdater) Check(ctx context.Context, currentVersion string) (updater.UpdateInfo, error) {
	return f.info, f.err
}

func (f *fakeUpdater) Install(ctx context.Context, info updater.UpdateInfo) (updater.UpdateResult, error) {
	f.installed = true
	if f.result.NewVersion == "" {
		f.result = updater.UpdateResult{PreviousVersion: info.CurrentVersion, NewVersion: info.LatestVersion, Path: "/tmp/pgsync", Duration: time.Second}
	}
	return f.result, f.err
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

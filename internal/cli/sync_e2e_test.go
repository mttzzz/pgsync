package cli_test

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mttzzz/pgsync/internal/cli"
	"github.com/mttzzz/pgsync/internal/config"
	"github.com/mttzzz/pgsync/internal/engine"
	"github.com/mttzzz/pgsync/internal/models"
)

func TestSyncResolvesDBNameViaInfisicalE2E(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell-stub e2e is POSIX-only")
	}

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".infisical.json"), []byte(`{"workspaceId":"x"}`), 0o600))

	binDir := filepath.Join(dir, "bin")
	require.NoError(t, os.Mkdir(binDir, 0o750))
	stub := filepath.Join(binDir, "infisical")
	require.NoError(t, os.WriteFile(stub, []byte("#!/bin/sh\necho \"POSTGRES_URL='postgresql://u:p@h:5432/e2e_db?sslmode=require'\"\n"), 0o700)) // #nosec G306 -- shell stub must be executable for exec.

	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Chdir(dir)

	cfgPath := filepath.Join(dir, "config.toml")
	cfg := config.Defaults()
	cfg.Remote = config.Connection{Host: "remote.example.com", Port: 5432, User: "u", Password: "p", SSLMode: "require"}
	cfg.Local = config.Connection{Host: "localhost", Port: 5432, User: "u", SSLMode: "disable"}
	require.NoError(t, config.Save(cfgPath, cfg))

	out := &bytes.Buffer{}
	stubEng := &capturingEngine{}
	app := cli.App{
		Out:           out,
		Err:           io.Discard,
		EngineFactory: func(*slog.Logger) (engine.Engine, error) { return stubEng, nil },
	}
	root := cli.NewRootCommand(app)
	root.SetArgs([]string{"sync", "--dry-run", "--config", cfgPath})
	require.NoError(t, root.ExecuteContext(context.Background()))
	assert.Equal(t, "e2e_db", stubEng.lastPlanDB)
	assert.Contains(t, out.String(), "e2e_db")
}

type capturingEngine struct{ lastPlanDB string }

func (c *capturingEngine) Plan(_ context.Context, opts engine.PlanOptions) (*models.SyncPlan, error) {
	c.lastPlanDB = opts.Database
	return &models.SyncPlan{Database: opts.Database}, nil
}

func (c *capturingEngine) Execute(_ context.Context, _ *models.SyncPlan, _ engine.ProgressObserver) (*models.SyncResult, error) {
	return &models.SyncResult{}, nil
}

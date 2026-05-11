package infisical_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mttzzz/pgsync/internal/secrets/infisical"
)

func TestResolveDBNameFailsWhenNoInfisicalJSON(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	r := infisical.Resolver{CWD: dir}
	_, err := r.ResolveDBName(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no .infisical.json found")
	assert.Contains(t, err.Error(), dir)
}

func TestResolveDBNameFindsInfisicalJSONInParent(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".infisical.json"), []byte(`{"workspaceId":"x"}`), 0o600))
	sub := filepath.Join(dir, "nested", "deep")
	require.NoError(t, os.MkdirAll(sub, 0o755))

	r := infisical.Resolver{CWD: sub}
	_, err := r.ResolveDBName(context.Background())
	require.Error(t, err)
	/* walk-up succeeded; we land in "not implemented" until later tasks. */
	assert.Contains(t, err.Error(), "not implemented")
}

func TestResolveDBNameFailsWhenInfisicalBinaryMissing(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".infisical.json"), []byte(`{}`), 0o600))

	r := infisical.Resolver{
		CWD:      dir,
		LookPath: func(string) (string, error) { return "", os.ErrNotExist },
	}
	_, err := r.ResolveDBName(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "'infisical' CLI not found in PATH")
}

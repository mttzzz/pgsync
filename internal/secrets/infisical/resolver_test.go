package infisical_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
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

	r := infisical.Resolver{
		CWD:      sub,
		LookPath: func(string) (string, error) { return "/usr/bin/infisical", nil },
		Run: func(context.Context, string, string, ...string) ([]byte, []byte, error) {
			return []byte("DB_DATABASE=walked_up_db\n"), nil, nil
		},
	}
	name, err := r.ResolveDBName(context.Background())
	/* walk-up succeeded; exec layer reached and returned the expected DB name. */
	require.NoError(t, err)
	assert.Equal(t, "walked_up_db", name)
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

func TestResolveDBNameExtractsFromPostgresURL(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".infisical.json"), []byte(`{}`), 0o600))

	r := infisical.Resolver{
		CWD:      dir,
		LookPath: func(string) (string, error) { return "/usr/bin/infisical", nil },
		Run: func(_ context.Context, gotDir, name string, args ...string) ([]byte, []byte, error) {
			assert.Equal(t, dir, gotDir)
			assert.Equal(t, "infisical", name)
			assert.Equal(t, []string{"export", "--env=dev", "--format=dotenv", "--silent"}, args)
			return []byte("POSTGRES_URL='postgresql://u:p@h:5432/ai_pushka_biz?sslmode=require'\n"), nil, nil
		},
	}
	name, err := r.ResolveDBName(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "ai_pushka_biz", name)
}

func TestResolveDBNameExtractsFromDBDatabase(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".infisical.json"), []byte(`{}`), 0o600))

	r := infisical.Resolver{
		CWD:      dir,
		LookPath: func(string) (string, error) { return "/usr/bin/infisical", nil },
		Run: func(context.Context, string, string, ...string) ([]byte, []byte, error) {
			return []byte("DB_DATABASE=masterm_pushka_biz\n"), nil, nil
		},
	}
	name, err := r.ResolveDBName(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "masterm_pushka_biz", name)
}

func TestResolveDBNamePostgresURLBeatsDBDatabase(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".infisical.json"), []byte(`{}`), 0o600))

	r := infisical.Resolver{
		CWD:      dir,
		LookPath: func(string) (string, error) { return "/usr/bin/infisical", nil },
		Run: func(context.Context, string, string, ...string) ([]byte, []byte, error) {
			return []byte(strings.Join([]string{
				"DB_DATABASE=ignored",
				`POSTGRES_URL="postgres://u@h/from_url"`,
			}, "\n") + "\n"), nil, nil
		},
	}
	name, err := r.ResolveDBName(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "from_url", name)
}

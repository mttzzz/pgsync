package infisical_test

import (
	"context"
	"errors"
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

func TestResolveDBNameFailsWhenInfisicalReturnsError(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".infisical.json"), []byte(`{}`), 0o600))

	r := infisical.Resolver{
		CWD:      dir,
		LookPath: func(string) (string, error) { return "/usr/bin/infisical", nil },
		Run: func(context.Context, string, string, ...string) ([]byte, []byte, error) {
			return nil, []byte("Unauthorized: token expired"), errors.New("exit status 1")
		},
	}
	_, err := r.ResolveDBName(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "infisical export failed")
	assert.Contains(t, err.Error(), "Unauthorized: token expired")
}

func TestResolveDBNameFailsWhenNoDBVars(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".infisical.json"), []byte(`{}`), 0o600))

	r := infisical.Resolver{
		CWD:      dir,
		LookPath: func(string) (string, error) { return "/usr/bin/infisical", nil },
		Run: func(context.Context, string, string, ...string) ([]byte, []byte, error) {
			return []byte("SOME_OTHER=value\n"), nil, nil
		},
	}
	_, err := r.ResolveDBName(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot resolve DB name from Infisical")
}

func TestResolveDBNameFailsOnBadPostgresURL(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".infisical.json"), []byte(`{}`), 0o600))

	r := infisical.Resolver{
		CWD:      dir,
		LookPath: func(string) (string, error) { return "/usr/bin/infisical", nil },
		Run: func(context.Context, string, string, ...string) ([]byte, []byte, error) {
			return []byte("POSTGRES_URL='mysql://app@h/db'\n"), nil, nil
		},
	}
	_, err := r.ResolveDBName(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse POSTGRES_URL")
}

/*
 * TestResolveDBNameFailsOnUnparsableURL exercises the url.Parse error branch
 * inside dbFromPostgresURL by providing a URL with a control character that
 * url.Parse rejects before even checking the scheme.
 */
func TestResolveDBNameFailsOnUnparsableURL(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".infisical.json"), []byte(`{}`), 0o600))

	r := infisical.Resolver{
		CWD:      dir,
		LookPath: func(string) (string, error) { return "/usr/bin/infisical", nil },
		Run: func(context.Context, string, string, ...string) ([]byte, []byte, error) {
			/* A URL containing a raw control character (\x01) is rejected by url.Parse. */
			return []byte("POSTGRES_URL=postgres://h/db\x01bad\n"), nil, nil
		},
	}
	_, err := r.ResolveDBName(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse POSTGRES_URL")
}

/* TestResolveDBNameWithEmptyCWD exercises the os.Getwd() fallback path inside
 * findInfisicalRoot when Resolver.CWD is left empty. */
func TestResolveDBNameWithEmptyCWD(t *testing.T) {
	/* Not parallel: t.Chdir changes process working directory. */
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".infisical.json"), []byte(`{}`), 0o600))
	t.Chdir(dir)

	r := infisical.Resolver{
		/* CWD intentionally left empty — resolver must fall back to os.Getwd(). */
		LookPath: func(string) (string, error) { return "/usr/bin/infisical", nil },
		Run: func(context.Context, string, string, ...string) ([]byte, []byte, error) {
			return []byte("DB_DATABASE=cwd_fallback_db\n"), nil, nil
		},
	}
	name, err := r.ResolveDBName(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "cwd_fallback_db", name)
}

/*
 * TestResolveDBNameWithNilHooksUsesRealExec exercises the defaultRun and
 * real exec.LookPath branches by placing a fake `infisical` script on PATH
 * and leaving both LookPath and Run nil.
 */
func TestResolveDBNameWithNilHooksUsesRealExec(t *testing.T) {
	/* Not parallel: modifies PATH env var via t.Setenv. */
	binDir := t.TempDir()
	projDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(projDir, ".infisical.json"), []byte(`{}`), 0o600))

	/* Write a POSIX shell script that acts as infisical export. */
	script := "#!/bin/sh\nprintf 'DB_DATABASE=nil_hooks_db\\n'\n"
	scriptPath := filepath.Join(binDir, "infisical")
	require.NoError(t, os.WriteFile(scriptPath, []byte(script), 0o755))

	/* Prepend fake bin dir so exec.LookPath finds our stub. */
	origPath := os.Getenv("PATH")
	t.Setenv("PATH", binDir+":"+origPath)

	r := infisical.Resolver{CWD: projDir}
	name, err := r.ResolveDBName(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "nil_hooks_db", name)
}

/* TestParseDotenvEdgeCases drives parseDotenvLine branches that are only
 * reachable via the exported ResolveDBName path — specifically lines with
 * no '=' separator, empty keys, and inline hash comments. */
func TestParseDotenvEdgeCases(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".infisical.json"), []byte(`{}`), 0o600))

	r := infisical.Resolver{
		CWD:      dir,
		LookPath: func(string) (string, error) { return "/usr/bin/infisical", nil },
		Run: func(context.Context, string, string, ...string) ([]byte, []byte, error) {
			blob := strings.Join([]string{
				"# a comment line",        /* comment — skipped */
				"NO_EQUALS_HERE",          /* no '=' — skipped */
				"=missing_key",            /* empty key — skipped */
				"DB_DATABASE=real_db # x", /* inline hash comment stripped */
			}, "\n") + "\n"
			return []byte(blob), nil, nil
		},
	}
	name, err := r.ResolveDBName(context.Background())
	require.NoError(t, err)
	/* inline comment is stripped; only "real_db" should remain. */
	assert.Equal(t, "real_db", name)
}

package native

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mttzzz/pgsync/internal/pgdb"
)

func TestSchemaDumperDumpPreDataRunsPgDumpWithPlainSchemaOnly(t *testing.T) {
	t.Parallel()
	source := schemaSourceEndpoint("secret")
	connString, err := pgdb.BuildConnString(source)
	require.NoError(t, err)
	runner := &fakeCommandRunner{stdout: []byte("CREATE SCHEMA public;\n")}
	dumper := &SchemaDumper{Runner: runner, Locator: &fakePgtoolsLocator{dumpPath: "/usr/bin/pg_dump"}}

	dump, err := dumper.Dump(context.Background(), source, SchemaPreData)

	require.NoError(t, err)
	assert.Equal(t, "CREATE SCHEMA public;\n", dump)
	require.Len(t, runner.calls, 1)
	assert.Equal(t, fakeCommandCall{
		name: "/usr/bin/pg_dump",
		args: []string{
			"--schema-only",
			"--no-owner",
			"--no-acl",
			"--format=plain",
			"--section=pre-data",
			connString,
		},
	}, runner.calls[0])
}

func TestSchemaDumperStripsPgDumpMetaCommands(t *testing.T) {
	t.Parallel()
	runner := &fakeCommandRunner{stdout: []byte("\\restrict abc\nCREATE TABLE public.users(id integer);\n  \\unrestrict abc\n")}
	dumper := &SchemaDumper{Runner: runner, Locator: &fakePgtoolsLocator{dumpPath: "pg_dump"}}

	dump, err := dumper.Dump(context.Background(), schemaSourceEndpoint("secret"), SchemaPreData)

	require.NoError(t, err)
	assert.Equal(t, "CREATE TABLE public.users(id integer);\n", dump)
	assert.Equal(t, "SELECT 1;\n", stripPgDumpMetaCommands("\\connect db\nSELECT 1;\n"))
}

func TestSchemaDumperDumpPostDataAllowsEmptyDump(t *testing.T) {
	t.Parallel()
	source := schemaSourceEndpoint("secret")
	connString, err := pgdb.BuildConnString(source)
	require.NoError(t, err)
	runner := &fakeCommandRunner{}
	dumper := &SchemaDumper{Runner: runner, Locator: &fakePgtoolsLocator{dumpPath: "pg_dump"}}

	dump, err := dumper.Dump(context.Background(), source, SchemaPostData)

	require.NoError(t, err)
	assert.Empty(t, dump)
	require.Len(t, runner.calls, 1)
	assert.Equal(t, []string{
		"--schema-only",
		"--no-owner",
		"--no-acl",
		"--format=plain",
		"--section=post-data",
		connString,
	}, runner.calls[0].args)
}

func TestSchemaDumperDumpRejectsUnsupportedSection(t *testing.T) {
	t.Parallel()
	dumper := &SchemaDumper{Runner: &fakeCommandRunner{}, Locator: &fakePgtoolsLocator{dumpPath: "pg_dump"}}

	dump, err := dumper.Dump(context.Background(), schemaSourceEndpoint("secret"), SchemaSection("data"))

	require.Error(t, err)
	assert.Empty(t, dump)
	assert.Contains(t, err.Error(), "unsupported schema section")
}

func TestSchemaDumperDumpRequiresCollaborators(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		dumper *SchemaDumper
		want   string
	}{
		{name: "dumper", dumper: nil, want: "schema dumper is required"},
		{name: "runner", dumper: &SchemaDumper{Locator: &fakePgtoolsLocator{dumpPath: "pg_dump"}}, want: "schema dumper runner is required"},
		{name: "locator", dumper: &SchemaDumper{Runner: &fakeCommandRunner{}}, want: "schema dumper locator is required"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dump, err := tt.dumper.Dump(context.Background(), schemaSourceEndpoint("secret"), SchemaPreData)

			require.Error(t, err)
			assert.Empty(t, dump)
			assert.EqualError(t, err, tt.want)
		})
	}
}

func TestSchemaDumperDumpBuildConnectionStringError(t *testing.T) {
	t.Parallel()
	dumper := &SchemaDumper{Runner: &fakeCommandRunner{}, Locator: &fakePgtoolsLocator{dumpPath: "pg_dump"}}

	dump, err := dumper.Dump(context.Background(), pgdb.Endpoint{Port: 5432}, SchemaPreData)

	require.Error(t, err)
	assert.Empty(t, dump)
	assert.Contains(t, err.Error(), "build source connection string")
}

func TestSchemaDumperDumpLocateError(t *testing.T) {
	t.Parallel()
	locateErr := errors.New("missing pg_dump")
	dumper := &SchemaDumper{Runner: &fakeCommandRunner{}, Locator: &fakePgtoolsLocator{err: locateErr}}

	dump, err := dumper.Dump(context.Background(), schemaSourceEndpoint("secret"), SchemaPreData)

	require.Error(t, err)
	assert.ErrorIs(t, err, locateErr)
	assert.Empty(t, dump)
	assert.Contains(t, err.Error(), "locate pg_dump")
}

func TestSchemaDumperDumpCommandErrorRedactsPasswordEverywhere(t *testing.T) {
	t.Parallel()
	source := schemaSourceEndpoint("s3cr3t")
	connString, err := pgdb.BuildConnString(source)
	require.NoError(t, err)
	runner := &fakeCommandRunner{
		stderr: []byte("permission denied for " + connString + " password=s3cr3t plain s3cr3t"),
		err:    errors.New("pg_dump failed for " + connString + " with s3cr3t"),
	}
	dumper := &SchemaDumper{Runner: runner, Locator: &fakePgtoolsLocator{dumpPath: "pg_dump"}}

	dump, err := dumper.Dump(context.Background(), source, SchemaPreData)

	require.Error(t, err)
	assert.Empty(t, dump)
	assert.NotContains(t, err.Error(), "s3cr3t")
	assert.Contains(t, err.Error(), "xxxxx")
	assert.Contains(t, err.Error(), "stderr")
	assert.Contains(t, err.Error(), "permission denied")
}

func TestSchemaDumperDumpCommandErrorOmitsEmptyStderr(t *testing.T) {
	t.Parallel()
	runner := &fakeCommandRunner{err: errors.New("boom")}
	dumper := &SchemaDumper{Runner: runner, Locator: &fakePgtoolsLocator{dumpPath: "pg_dump"}}

	dump, err := dumper.Dump(context.Background(), schemaSourceEndpoint("secret"), SchemaPostData)

	require.Error(t, err)
	assert.Empty(t, dump)
	assert.NotContains(t, err.Error(), "stderr")
}

func TestSchemaDumperDumpPreDataRejectsEmptyDump(t *testing.T) {
	t.Parallel()
	runner := &fakeCommandRunner{stdout: []byte(" \n\t")}
	dumper := &SchemaDumper{Runner: runner, Locator: &fakePgtoolsLocator{dumpPath: "pg_dump"}}

	dump, err := dumper.Dump(context.Background(), schemaSourceEndpoint("secret"), SchemaPreData)

	require.Error(t, err)
	assert.Empty(t, dump)
	assert.EqualError(t, err, "pg_dump returned empty pre-data schema")
}

func TestSchemaDumperDumpArchiveWritesWholeSchemaAsCustomArchive(t *testing.T) {
	t.Parallel()
	source := schemaSourceEndpoint("secret")
	connString, err := pgdb.BuildConnString(source)
	require.NoError(t, err)
	runner := &fakeCommandRunner{}
	dumper := &SchemaDumper{Runner: runner, Locator: &fakePgtoolsLocator{dumpPath: "/usr/bin/pg_dump"}}

	path, removeArchive, err := dumper.DumpArchive(context.Background(), source)

	require.NoError(t, err)
	require.NotNil(t, removeArchive)
	t.Cleanup(removeArchive)
	assert.True(t, strings.HasPrefix(filepath.Base(path), "pgsync-schema-"), path)
	assert.Equal(t, ".dump", filepath.Ext(path))
	assert.FileExists(t, path)
	require.Len(t, runner.calls, 1)
	assert.Equal(t, fakeCommandCall{
		name: "/usr/bin/pg_dump",
		args: []string{
			"--schema-only",
			"--no-owner",
			"--no-acl",
			"--format=custom",
			"--file=" + path,
			connString,
		},
	}, runner.calls[0])
	for _, arg := range runner.calls[0].args {
		assert.False(t, strings.HasPrefix(arg, "--section"), "the archive must carry the whole schema, got %s", arg)
	}

	removeArchive()
	assert.NoFileExists(t, path)
}

func TestSchemaDumperDumpArchiveRemovesFileWhenPgDumpFails(t *testing.T) {
	t.Parallel()
	source := schemaSourceEndpoint("s3cr3t")
	connString, err := pgdb.BuildConnString(source)
	require.NoError(t, err)
	runner := &fakeCommandRunner{
		stderr: []byte("permission denied for " + connString + " password=s3cr3t plain s3cr3t"),
		err:    errors.New("pg_dump failed for " + connString + " with s3cr3t"),
	}
	dumper := &SchemaDumper{Runner: runner, Locator: &fakePgtoolsLocator{dumpPath: "pg_dump"}}

	path, removeArchive, err := dumper.DumpArchive(context.Background(), source)

	require.Error(t, err)
	assert.Empty(t, path)
	assert.Nil(t, removeArchive)
	assert.NotContains(t, err.Error(), "s3cr3t")
	assert.Contains(t, err.Error(), "xxxxx")
	assert.Contains(t, err.Error(), "dump schema archive with pg_dump failed")
	assert.Contains(t, err.Error(), "stderr")
	require.Len(t, runner.calls, 1)
	archiveArg := runner.calls[0].args[len(runner.calls[0].args)-2]
	require.True(t, strings.HasPrefix(archiveArg, "--file="), archiveArg)
	assert.NoFileExists(t, strings.TrimPrefix(archiveArg, "--file="))
}

func TestSchemaDumperDumpArchiveRequiresCollaborators(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		dumper *SchemaDumper
		want   string
	}{
		{name: "dumper", dumper: nil, want: "schema dumper is required"},
		{name: "runner", dumper: &SchemaDumper{Locator: &fakePgtoolsLocator{dumpPath: "pg_dump"}}, want: "schema dumper runner is required"},
		{name: "locator", dumper: &SchemaDumper{Runner: &fakeCommandRunner{}}, want: "schema dumper locator is required"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			path, removeArchive, err := tt.dumper.DumpArchive(context.Background(), schemaSourceEndpoint("secret"))

			assert.EqualError(t, err, tt.want)
			assert.Empty(t, path)
			assert.Nil(t, removeArchive)
		})
	}
}

func TestSchemaDumperDumpArchiveSetupErrors(t *testing.T) {
	t.Parallel()
	locateErr := errors.New("missing pg_dump")

	_, _, err := (&SchemaDumper{Runner: &fakeCommandRunner{}, Locator: &fakePgtoolsLocator{dumpPath: "pg_dump"}}).
		DumpArchive(context.Background(), pgdb.Endpoint{Port: 5432})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "build source connection string")

	_, _, err = (&SchemaDumper{Runner: &fakeCommandRunner{}, Locator: &fakePgtoolsLocator{err: locateErr}}).
		DumpArchive(context.Background(), schemaSourceEndpoint("secret"))
	require.Error(t, err)
	assert.ErrorIs(t, err, locateErr)
	assert.Contains(t, err.Error(), "locate pg_dump")
}

func TestSchemaDumperDumpArchiveReportsUnwritableTempDir(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")
	t.Setenv("TMPDIR", missing)
	t.Setenv("TMP", missing)
	t.Setenv("TEMP", missing)
	runner := &fakeCommandRunner{}
	dumper := &SchemaDumper{Runner: runner, Locator: &fakePgtoolsLocator{dumpPath: "pg_dump"}}

	path, removeArchive, err := dumper.DumpArchive(context.Background(), schemaSourceEndpoint("secret"))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "create schema archive file")
	assert.Empty(t, path)
	assert.Nil(t, removeArchive)
	assert.Empty(t, runner.calls)
}

func TestArchiveRestorerRestoreRunsPgRestoreForSectionInParallel(t *testing.T) {
	t.Parallel()
	target := schemaSourceEndpoint("secret")
	connString, err := pgdb.BuildConnString(target)
	require.NoError(t, err)
	runner := &fakeCommandRunner{}
	restorer := &ArchiveRestorer{Runner: runner, Locator: &fakePgtoolsLocator{restorePath: "/usr/bin/pg_restore"}}

	err = restorer.Restore(context.Background(), target, "/tmp/schema.dump", SchemaPostData, 5)

	require.NoError(t, err)
	require.Len(t, runner.calls, 1)
	assert.Equal(t, fakeCommandCall{
		name: "/usr/bin/pg_restore",
		args: []string{
			"--no-owner",
			"--no-acl",
			"--exit-on-error",
			"--section=post-data",
			"--jobs=5",
			"--dbname=" + connString,
			"/tmp/schema.dump",
		},
	}, runner.calls[0])
}

func TestArchiveRestorerRestoreRunsAtLeastOneJob(t *testing.T) {
	t.Parallel()
	for _, jobs := range []int{-3, 0, 1} {
		runner := &fakeCommandRunner{}
		restorer := &ArchiveRestorer{Runner: runner, Locator: &fakePgtoolsLocator{restorePath: "pg_restore"}}

		err := restorer.Restore(context.Background(), schemaSourceEndpoint("secret"), "schema.dump", SchemaPostData, jobs)

		require.NoError(t, err)
		require.Len(t, runner.calls, 1)
		assert.Contains(t, runner.calls[0].args, "--jobs=1", "jobs=%d", jobs)
	}
}

func TestArchiveRestorerRestoreRejectsUnsupportedSection(t *testing.T) {
	t.Parallel()
	runner := &fakeCommandRunner{}
	restorer := &ArchiveRestorer{Runner: runner, Locator: &fakePgtoolsLocator{restorePath: "pg_restore"}}

	err := restorer.Restore(context.Background(), schemaSourceEndpoint("secret"), "schema.dump", SchemaSection("data"), 2)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported schema section")
	assert.Empty(t, runner.calls)
}

func TestArchiveRestorerRestoreRequiresCollaborators(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		restorer *ArchiveRestorer
		want     string
	}{
		{name: "restorer", restorer: nil, want: "archive restorer is required"},
		{name: "runner", restorer: &ArchiveRestorer{Locator: &fakePgtoolsLocator{restorePath: "pg_restore"}}, want: "archive restorer runner is required"},
		{name: "locator", restorer: &ArchiveRestorer{Runner: &fakeCommandRunner{}}, want: "archive restorer locator is required"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.restorer.Restore(context.Background(), schemaSourceEndpoint("secret"), "schema.dump", SchemaPostData, 2)

			assert.EqualError(t, err, tt.want)
		})
	}
}

func TestArchiveRestorerRestoreSetupErrors(t *testing.T) {
	t.Parallel()
	locateErr := errors.New("missing pg_restore")

	err := (&ArchiveRestorer{Runner: &fakeCommandRunner{}, Locator: &fakePgtoolsLocator{restorePath: "pg_restore"}}).
		Restore(context.Background(), pgdb.Endpoint{Port: 5432}, "schema.dump", SchemaPostData, 2)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "build target connection string")

	runner := &fakeCommandRunner{}
	err = (&ArchiveRestorer{Runner: runner, Locator: &fakePgtoolsLocator{restoreErr: locateErr}}).
		Restore(context.Background(), schemaSourceEndpoint("secret"), "schema.dump", SchemaPostData, 2)
	require.Error(t, err)
	assert.ErrorIs(t, err, locateErr)
	assert.Contains(t, err.Error(), "locate pg_restore")
	assert.Empty(t, runner.calls)
}

func TestArchiveRestorerRestoreCommandErrorRedactsPasswordEverywhere(t *testing.T) {
	t.Parallel()
	target := schemaSourceEndpoint("s3cr3t")
	connString, err := pgdb.BuildConnString(target)
	require.NoError(t, err)
	runner := &fakeCommandRunner{
		stderr: []byte("pg_restore: error: deadlock detected for " + connString + " password=s3cr3t plain s3cr3t"),
		err:    errors.New("pg_restore failed for " + connString + " with s3cr3t"),
	}
	restorer := &ArchiveRestorer{Runner: runner, Locator: &fakePgtoolsLocator{restorePath: "pg_restore"}}

	err = restorer.Restore(context.Background(), target, "schema.dump", SchemaPostData, 4)

	require.Error(t, err)
	assert.NotContains(t, err.Error(), "s3cr3t")
	assert.Contains(t, err.Error(), "xxxxx")
	assert.Contains(t, err.Error(), "pg_restore failed")
	assert.Contains(t, err.Error(), "stderr")
	assert.Contains(t, err.Error(), "deadlock detected")
}

func TestArchiveRestorerRestoreCommandErrorOmitsEmptyStderr(t *testing.T) {
	t.Parallel()
	runner := &fakeCommandRunner{err: errors.New("boom")}
	restorer := &ArchiveRestorer{Runner: runner, Locator: &fakePgtoolsLocator{restorePath: "pg_restore"}}

	err := restorer.Restore(context.Background(), schemaSourceEndpoint("secret"), "schema.dump", SchemaPostData, 2)

	require.Error(t, err)
	assert.EqualError(t, err, "pg_restore failed: boom")
}

func TestPgToolsReachProxiedEndpointThroughLocalTunnel(t *testing.T) {
	t.Parallel()
	proxied := schemaSourceEndpoint("secret")
	proxied.ProxyURL = "socks5://127.0.0.1:1"
	directConnString, err := pgdb.BuildConnString(schemaSourceEndpoint("secret"))
	require.NoError(t, err)

	dumpRunner := &fakeCommandRunner{}
	dumper := &SchemaDumper{Runner: dumpRunner, Locator: &fakePgtoolsLocator{dumpPath: "pg_dump"}}
	_, err = dumper.Dump(context.Background(), proxied, SchemaPostData)
	require.NoError(t, err)

	restoreRunner := &fakeCommandRunner{}
	restorer := &ArchiveRestorer{Runner: restoreRunner, Locator: &fakePgtoolsLocator{restorePath: "pg_restore"}}
	require.NoError(t, restorer.Restore(context.Background(), proxied, "schema.dump", SchemaPostData, 2))

	for name, runner := range map[string]*fakeCommandRunner{"pg_dump": dumpRunner, "pg_restore": restoreRunner} {
		require.Len(t, runner.calls, 1, name)
		joined := strings.Join(runner.calls[0].args, " ")
		assert.NotContains(t, joined, "remote.example.com", name)
		assert.NotContains(t, joined, directConnString, name)
		assert.Contains(t, joined, "@127.0.0.1:", name)
	}
}

func TestPgToolsReportBrokenProxy(t *testing.T) {
	t.Parallel()
	broken := schemaSourceEndpoint("secret")
	broken.ProxyURL = "ftp://proxy.example.com:21"
	dumpRunner := &fakeCommandRunner{}
	restoreRunner := &fakeCommandRunner{}

	_, dumpErr := (&SchemaDumper{Runner: dumpRunner, Locator: &fakePgtoolsLocator{dumpPath: "pg_dump"}}).
		Dump(context.Background(), broken, SchemaPreData)
	restoreErr := (&ArchiveRestorer{Runner: restoreRunner, Locator: &fakePgtoolsLocator{restorePath: "pg_restore"}}).
		Restore(context.Background(), broken, "schema.dump", SchemaPostData, 2)

	require.Error(t, dumpErr)
	assert.Contains(t, dumpErr.Error(), "init proxy tunnel")
	require.Error(t, restoreErr)
	assert.Contains(t, restoreErr.Error(), "init proxy tunnel")
	assert.Empty(t, dumpRunner.calls)
	assert.Empty(t, restoreRunner.calls)
}

func schemaSourceEndpoint(password string) pgdb.Endpoint {
	return pgdb.Endpoint{
		Host:     "remote.example.com",
		Port:     5432,
		User:     "app",
		Password: password,
		Database: "appdb",
		SSLMode:  "require",
	}
}

type fakeCommandCall struct {
	name string
	args []string
	env  []string
}

type fakeCommandRunner struct {
	calls  []fakeCommandCall
	stdout []byte
	stderr []byte
	err    error
}

func (r *fakeCommandRunner) Run(_ context.Context, name string, args []string, env []string) ([]byte, []byte, error) {
	r.calls = append(r.calls, fakeCommandCall{
		name: name,
		args: append([]string(nil), args...),
		env:  append([]string(nil), env...),
	})
	return append([]byte(nil), r.stdout...), append([]byte(nil), r.stderr...), r.err
}

type fakePgtoolsLocator struct {
	dumpPath    string
	err         error
	restorePath string
	restoreErr  error
}

func (l *fakePgtoolsLocator) PgDump() (string, error) {
	return l.dumpPath, l.err
}

func (l *fakePgtoolsLocator) PgRestore() (string, error) {
	return l.restorePath, l.restoreErr
}

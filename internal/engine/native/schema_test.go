package native

import (
	"context"
	"errors"
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
	dumpPath string
	err      error
}

func (l *fakePgtoolsLocator) PgDump() (string, error) {
	return l.dumpPath, l.err
}

func (l *fakePgtoolsLocator) PgRestore() (string, error) {
	return "", errors.New("unexpected pg_restore")
}

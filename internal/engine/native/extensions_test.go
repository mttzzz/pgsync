package native

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mttzzz/pgsync/internal/pgdb"
)

func TestSchemaExtensionsParsesCreateExtensionStatements(t *testing.T) {
	t.Parallel()
	sql := `
CREATE EXTENSION IF NOT EXISTS vector WITH SCHEMA public;
CREATE EXTENSION hstore;
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS vector;
`

	got := schemaExtensions(sql)

	assert.Equal(t, []string{"hstore", "uuid-ossp", "vector"}, got)
}

func TestExtensionCheckerReturnsBeforeConnectingWhenNoExtensions(t *testing.T) {
	t.Parallel()
	connector := &nativeFakeConnector{}
	checker := &ExtensionChecker{Connector: connector}

	err := checker.CheckPreData(context.Background(), targetLocalConnection(""), "CREATE TABLE public.users(id int);")

	require.NoError(t, err)
	assert.Empty(t, connector.calls)
}

func TestExtensionCheckerFailsBeforeResetWhenExtensionMissing(t *testing.T) {
	t.Parallel()
	conn := &nativeFakeConn{queryResults: []nativeQueryResult{{rows: [][]any{{"hstore"}}}}}
	checker := &ExtensionChecker{Connector: &nativeFakeConnector{conns: []pgdb.CopyConn{conn}}}

	err := checker.CheckPreData(context.Background(), targetLocalConnection("maintenance"), "CREATE EXTENSION vector; CREATE EXTENSION hstore;")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing local PostgreSQL extensions")
	assert.Contains(t, err.Error(), "vector")
	assert.Equal(t, 1, conn.closeCount)
}

func TestExtensionCheckerAllowsAvailableExtensions(t *testing.T) {
	t.Parallel()
	conn := &nativeFakeConn{queryResults: []nativeQueryResult{{rows: [][]any{{"vector"}}}}}
	checker := &ExtensionChecker{Connector: &nativeFakeConnector{conns: []pgdb.CopyConn{conn}}}

	err := checker.CheckPreData(context.Background(), targetLocalConnection(""), "CREATE EXTENSION IF NOT EXISTS vector;")

	require.NoError(t, err)
	assert.Equal(t, 1, conn.closeCount)
}

func TestExtensionCheckerWrapsQueryAndRowsErrors(t *testing.T) {
	t.Parallel()
	queryErr := errors.New("query failed")
	conn := &nativeFakeConn{queryResults: []nativeQueryResult{{err: queryErr}}}
	checker := &ExtensionChecker{Connector: &nativeFakeConnector{conns: []pgdb.CopyConn{conn}}}

	err := checker.CheckPreData(context.Background(), targetLocalConnection(""), "CREATE EXTENSION vector;")

	require.Error(t, err)
	assert.ErrorIs(t, err, queryErr)
}

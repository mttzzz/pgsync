package helpers

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mttzzz/pgsync/internal/models"
)

func TestParseTableStringUsesPublicDefaultSchema(t *testing.T) {
	t.Parallel()

	table, err := parseTable("users")

	require.NoError(t, err)
	assert.Equal(t, models.Table{Schema: "public", Name: "users"}, table)
}

func TestParseTableStringKeepsExplicitSchema(t *testing.T) {
	t.Parallel()

	table, err := parseTable("tenant.orders")

	require.NoError(t, err)
	assert.Equal(t, models.Table{Schema: "tenant", Name: "orders"}, table)
}

func TestParseTableModelRejectsEmptyName(t *testing.T) {
	t.Parallel()

	_, err := parseTable(models.Table{Schema: "public"})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "table name is required")
}

func TestTableRowCountQueryQuotesTable(t *testing.T) {
	t.Parallel()

	query, err := tableRowCountQuery(models.Table{Schema: "odd schema", Name: `quoted"table`})

	require.NoError(t, err)
	assert.Equal(t, `SELECT count(*)::bigint FROM "odd schema"."quoted""table"`, query)
}

func TestTableChecksumQueryUsesDeterministicJSONBAggregation(t *testing.T) {
	t.Parallel()

	query, err := tableChecksumQuery(models.Table{Schema: "public", Name: "users"})

	require.NoError(t, err)
	assert.Contains(t, query, `to_jsonb(t) AS row_json`)
	assert.Contains(t, query, `string_agg(row_json::text, E'\n' ORDER BY row_json::text)`)
	assert.Contains(t, query, `FROM "public"."users" AS t`)
}

func TestTableMaxColumnValueQueryQuotesColumn(t *testing.T) {
	t.Parallel()

	query, err := tableMaxColumnValueQuery(models.Table{Schema: "public", Name: "users"}, `id"part`)

	require.NoError(t, err)
	assert.Equal(t, `SELECT COALESCE(max("id""part"), 0)::bigint FROM "public"."users"`, query)
}

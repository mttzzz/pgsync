package pgdb_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mttzzz/pgsync/internal/models"
	"github.com/mttzzz/pgsync/internal/pgdb"
)

func TestQuoteIdent(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"users":     `"users"`,
		`user"name`: `"user""name"`,
	}

	for raw, want := range cases {
		got, err := pgdb.QuoteIdent(raw)
		require.NoError(t, err)
		assert.Equal(t, want, got)
	}
}

func TestQuoteIdentRejectsBlank(t *testing.T) {
	t.Parallel()

	_, err := pgdb.QuoteIdent(" \t")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "identifier")
}

func TestQuoteQualified(t *testing.T) {
	t.Parallel()

	got, err := pgdb.QuoteQualified(models.Table{Schema: "public", Name: "orders"})

	require.NoError(t, err)
	assert.Equal(t, `"public"."orders"`, got)
}

func TestQuoteQualifiedRejectsEmptyParts(t *testing.T) {
	t.Parallel()
	cases := map[string]models.Table{
		"schema": {Name: "orders"},
		"name":   {Schema: "public"},
	}

	for name, table := range cases {
		_, err := pgdb.QuoteQualified(table)
		require.Error(t, err, name)
		assert.Contains(t, err.Error(), "quote", name)
	}
}

func TestParseTableName(t *testing.T) {
	t.Parallel()
	cases := []struct {
		raw  string
		want models.Table
	}{
		{raw: "users", want: models.Table{Schema: "public", Name: "users"}},
		{raw: "public.users", want: models.Table{Schema: "public", Name: "users"}},
		{raw: " audit.logs ", want: models.Table{Schema: "audit", Name: "logs"}},
	}

	for _, tt := range cases {
		got, err := pgdb.ParseTableName(tt.raw, "public")
		require.NoError(t, err)
		assert.Equal(t, tt.want, got)
	}
}

func TestParseTableNameRejectsInvalidInput(t *testing.T) {
	t.Parallel()
	cases := []string{"", " \t", "a.b.c", ".users", "public."}

	for _, raw := range cases {
		_, err := pgdb.ParseTableName(raw, "public")
		require.Error(t, err, raw)
	}
}

func TestParseTableNameRejectsUnqualifiedNameWithoutDefaultSchema(t *testing.T) {
	t.Parallel()

	_, err := pgdb.ParseTableName("users", "")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "schema")
}

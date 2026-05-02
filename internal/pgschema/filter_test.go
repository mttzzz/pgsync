package pgschema_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mttzzz/pgsync/internal/models"
	"github.com/mttzzz/pgsync/internal/pgschema"
)

func TestFKClosureLinear(t *testing.T) {
	t.Parallel()
	all := []models.Table{tbl("users"), tbl("orders"), tbl("items"), tbl("unrelated")}
	deps := []models.FKDep{
		dep("orders", "users"),
		dep("items", "orders"),
	}
	got, err := pgschema.FKClosure(all, deps, []string{"items"})
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"users", "orders", "items"}, names(got))
}

func TestFKClosureSchemaQualifiedRequest(t *testing.T) {
	t.Parallel()
	all := []models.Table{tbl("a"), tbl("b")}
	deps := []models.FKDep{dep("b", "a")}
	got, err := pgschema.FKClosure(all, deps, []string{"public.b"})
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"a", "b"}, names(got))
}

func TestFKClosureUnknownTable(t *testing.T) {
	t.Parallel()
	all := []models.Table{tbl("a")}
	_, err := pgschema.FKClosure(all, nil, []string{"ghost"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ghost")
}

func TestFKClosureAlreadyClosed(t *testing.T) {
	t.Parallel()
	all := []models.Table{tbl("a"), tbl("b")}
	deps := []models.FKDep{dep("b", "a")}
	got, err := pgschema.FKClosure(all, deps, []string{"a", "b"})
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"a", "b"}, names(got))
}

func TestFKClosureEmptyRequestReturnsAll(t *testing.T) {
	t.Parallel()
	all := []models.Table{tbl("a"), tbl("b")}
	got, err := pgschema.FKClosure(all, nil, nil)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"a", "b"}, names(got))
}

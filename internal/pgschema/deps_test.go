package pgschema_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mttzzz/pgsync/internal/models"
	"github.com/mttzzz/pgsync/internal/pgschema"
)

func tbl(name string) models.Table {
	return models.Table{Schema: "public", Name: name}
}

func dep(from, to string) models.FKDep {
	return models.FKDep{From: tbl(from), To: tbl(to)}
}

func TestTopoSortLinear(t *testing.T) {
	t.Parallel()
	tables := []models.Table{tbl("a"), tbl("b"), tbl("c")}
	deps := []models.FKDep{dep("c", "b"), dep("b", "a")}
	got, err := pgschema.TopoSort(tables, deps)
	require.NoError(t, err)
	assert.Equal(t, []string{"a", "b", "c"}, names(got))
}

func TestTopoSortDiamond(t *testing.T) {
	t.Parallel()
	tables := []models.Table{tbl("a"), tbl("b"), tbl("c"), tbl("d")}
	deps := []models.FKDep{
		dep("d", "b"), dep("d", "c"),
		dep("b", "a"), dep("c", "a"),
	}
	got, err := pgschema.TopoSort(tables, deps)
	require.NoError(t, err)
	idx := map[string]int{}
	for i, table := range got {
		idx[table.Name] = i
	}
	assert.Less(t, idx["a"], idx["b"])
	assert.Less(t, idx["a"], idx["c"])
	assert.Less(t, idx["b"], idx["d"])
	assert.Less(t, idx["c"], idx["d"])
}

func TestTopoSortSelfReference(t *testing.T) {
	t.Parallel()
	tables := []models.Table{tbl("categories")}
	deps := []models.FKDep{dep("categories", "categories")}
	got, err := pgschema.TopoSort(tables, deps)
	require.NoError(t, err)
	assert.Equal(t, []string{"categories"}, names(got))
}

func TestTopoSortRealCycleErrors(t *testing.T) {
	t.Parallel()
	tables := []models.Table{tbl("a"), tbl("b")}
	deps := []models.FKDep{dep("a", "b"), dep("b", "a")}
	_, err := pgschema.TopoSort(tables, deps)
	require.Error(t, err)
	var cycleErr *pgschema.CycleError
	require.ErrorAs(t, err, &cycleErr)
	assert.NotEmpty(t, cycleErr.Cycle)
	assert.Contains(t, cycleErr.Error(), "FK cycle")
}

func TestTopoSortIgnoresUnknownAndDuplicateDep(t *testing.T) {
	t.Parallel()
	tables := []models.Table{tbl("a"), tbl("b")}
	deps := []models.FKDep{dep("a", "ghost"), dep("ghost", "a"), dep("b", "a"), dep("b", "a")}
	got, err := pgschema.TopoSort(tables, deps)
	require.NoError(t, err)
	assert.Equal(t, []string{"a", "b"}, names(got))
}

func names(tables []models.Table) []string {
	out := make([]string, len(tables))
	for i, table := range tables {
		out[i] = table.Name
	}
	return out
}

package pgschema

import (
	"fmt"
	"slices"
	"strings"

	"github.com/mttzzz/pgsync/internal/models"
)

/* FKClosure returns requested tables plus every table reachable by following
 * FK edges from child to parent. Empty requested means all tables.
 */
func FKClosure(all []models.Table, deps []models.FKDep, requested []string) ([]models.Table, error) {
	if len(requested) == 0 {
		return cloneTables(all), nil
	}

	selected, err := seedTables(indexTables(all), requested)
	if err != nil {
		return nil, err
	}
	expandParents(selected, parentIndex(deps))
	return sortedTableValues(selected), nil
}

func cloneTables(in []models.Table) []models.Table {
	out := make([]models.Table, len(in))
	copy(out, in)
	return out
}

func indexTables(all []models.Table) map[string]models.Table {
	byName := make(map[string]models.Table, len(all)*2)
	for _, table := range all {
		byName[table.Name] = table
		byName[table.Schema+"."+table.Name] = table
	}
	return byName
}

func seedTables(byName map[string]models.Table, requested []string) (map[string]models.Table, error) {
	selected := make(map[string]models.Table, len(requested))
	for _, raw := range requested {
		table, ok := byName[raw]
		if !ok {
			return nil, fmt.Errorf("table not found: %s", raw)
		}
		selected[table.QualifiedName()] = table
	}
	return selected, nil
}

func parentIndex(deps []models.FKDep) map[string][]models.Table {
	parents := make(map[string][]models.Table, len(deps))
	for _, dep := range deps {
		parents[dep.From.QualifiedName()] = append(parents[dep.From.QualifiedName()], dep.To)
	}
	return parents
}

func expandParents(selected map[string]models.Table, parents map[string][]models.Table) {
	queue := make([]models.Table, 0, len(selected))
	for _, table := range selected {
		queue = append(queue, table)
	}
	for len(queue) > 0 {
		head := queue[0]
		queue = queue[1:]
		queue = appendMissingParents(queue, selected, parents[head.QualifiedName()])
	}
}

func appendMissingParents(queue []models.Table, selected map[string]models.Table, parents []models.Table) []models.Table {
	for _, parent := range parents {
		key := parent.QualifiedName()
		if _, ok := selected[key]; !ok {
			selected[key] = parent
			queue = append(queue, parent)
		}
	}
	return queue
}

func sortedTableValues(selected map[string]models.Table) []models.Table {
	out := make([]models.Table, 0, len(selected))
	for _, table := range selected {
		out = append(out, table)
	}
	slices.SortFunc(out, func(a, b models.Table) int {
		return strings.Compare(a.QualifiedName(), b.QualifiedName())
	})
	return out
}

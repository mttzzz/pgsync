// Package pgschema isolates PostgreSQL schema graph logic.
package pgschema

import (
	"fmt"
	"slices"

	"github.com/mttzzz/pgsync/internal/models"
)

/* CycleError reports a real foreign-key cycle across distinct tables. */
type CycleError struct {
	Cycle []string
}

/* Error returns a human-readable cycle description. */
func (e *CycleError) Error() string {
	return fmt.Sprintf("FK cycle detected: %v", e.Cycle)
}

/* TopoSort returns tables in dependency-first order: parents referenced by
 * foreign keys come before children. Self-FKs are ignored. Real cycles across
 * distinct nodes return *CycleError.
 */
func TopoSort(tables []models.Table, deps []models.FKDep) ([]models.Table, error) {
	graph := newTopoGraph(tables)
	graph.addDeps(deps)

	out := graph.sorted()
	if len(out) != len(tables) {
		return nil, &CycleError{Cycle: graph.stuck()}
	}
	return out, nil
}

type topoGraph struct {
	byKey     map[string]models.Table
	indegrees map[string]int
	children  map[string][]string
	seenEdges map[string]struct{}
}

func newTopoGraph(tables []models.Table) *topoGraph {
	graph := &topoGraph{
		byKey:     make(map[string]models.Table, len(tables)),
		indegrees: make(map[string]int, len(tables)),
		children:  make(map[string][]string, len(tables)),
		seenEdges: make(map[string]struct{}),
	}
	for _, table := range tables {
		key := table.QualifiedName()
		graph.byKey[key] = table
		graph.indegrees[key] = 0
	}
	return graph
}

func (g *topoGraph) addDeps(deps []models.FKDep) {
	for _, dep := range deps {
		g.addDep(dep)
	}
}

func (g *topoGraph) addDep(dep models.FKDep) {
	fromKey := dep.From.QualifiedName()
	toKey := dep.To.QualifiedName()
	if g.skipDep(fromKey, toKey) {
		return
	}
	g.seenEdges[topoEdgeKey(toKey, fromKey)] = struct{}{}
	g.children[toKey] = append(g.children[toKey], fromKey)
	g.indegrees[fromKey]++
}

func (g *topoGraph) skipDep(fromKey, toKey string) bool {
	if fromKey == toKey {
		return true
	}
	if _, ok := g.byKey[fromKey]; !ok {
		return true
	}
	if _, ok := g.byKey[toKey]; !ok {
		return true
	}
	_, duplicate := g.seenEdges[topoEdgeKey(toKey, fromKey)]
	return duplicate
}

func topoEdgeKey(parent, child string) string {
	return parent + "\x00" + child
}

func (g *topoGraph) sorted() []models.Table {
	ready := g.ready()
	out := make([]models.Table, 0, len(g.byKey))
	for len(ready) > 0 {
		head := ready[0]
		ready = ready[1:]
		out = append(out, g.byKey[head])
		ready = g.releaseChildren(head, ready)
	}
	return out
}

func (g *topoGraph) ready() []string {
	ready := make([]string, 0, len(g.indegrees))
	for key, degree := range g.indegrees {
		if degree == 0 {
			ready = append(ready, key)
		}
	}
	slices.Sort(ready)
	return ready
}

func (g *topoGraph) releaseChildren(parent string, ready []string) []string {
	next := g.children[parent]
	slices.Sort(next)
	for _, child := range next {
		g.indegrees[child]--
		if g.indegrees[child] == 0 {
			ready = append(ready, child)
			slices.Sort(ready)
		}
	}
	return ready
}

func (g *topoGraph) stuck() []string {
	stuck := make([]string, 0)
	for key, degree := range g.indegrees {
		if degree > 0 {
			stuck = append(stuck, key)
		}
	}
	slices.Sort(stuck)
	return stuck
}

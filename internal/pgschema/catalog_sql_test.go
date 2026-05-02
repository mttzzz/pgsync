package pgschema_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/mttzzz/pgsync/internal/pgschema"
)

func TestListDatabasesSQLFiltersTemplatesAndSystemDatabases(t *testing.T) {
	t.Parallel()
	sql := compactSQL(pgschema.ListDatabasesSQL())

	assert.Contains(t, sql, "FROM pg_catalog.pg_database AS d")
	assert.Contains(t, sql, "pg_catalog.pg_database_size(d.datname)")
	assert.Contains(t, sql, "pg_catalog.pg_get_userbyid(d.datdba)")
	assert.Contains(t, sql, "WHERE d.datallowconn")
	assert.Contains(t, sql, "AND NOT d.datistemplate")
	assert.Contains(t, sql, "AND d.datname <> 'postgres'")
	assert.Contains(t, sql, "ORDER BY d.datname")
}

func TestListDatabasesIncludingSystemSQLKeepsTemplateFilter(t *testing.T) {
	t.Parallel()
	sql := compactSQL(pgschema.ListDatabasesIncludingSystemSQL())

	assert.Contains(t, sql, "FROM pg_catalog.pg_database AS d")
	assert.Contains(t, sql, "AND NOT d.datistemplate")
	assert.NotContains(t, sql, "d.datname <> 'postgres'")
}

func TestListTablesSQLUsesClassNamespaceAndUserSchemaFilters(t *testing.T) {
	t.Parallel()
	sql := compactSQL(pgschema.ListTablesSQL())

	assert.Contains(t, sql, "FROM pg_catalog.pg_class AS c")
	assert.Contains(t, sql, "JOIN pg_catalog.pg_namespace AS n")
	assert.Contains(t, sql, "pg_catalog.pg_total_relation_size(c.oid)")
	assert.Contains(t, sql, "GREATEST(c.reltuples, 0)::bigint")
	assert.Contains(t, sql, "c.relkind IN ('r', 'p')")
	assert.Contains(t, sql, "n.nspname <> 'pg_catalog'")
	assert.Contains(t, sql, "n.nspname <> 'information_schema'")
	assert.Contains(t, sql, "n.nspname NOT LIKE 'pg_%'")
}

func TestListFKDepsSQLUsesConstraintsAndMapsChildToParent(t *testing.T) {
	t.Parallel()
	sql := compactSQL(pgschema.ListFKDepsSQL())

	assert.Contains(t, sql, "FROM pg_catalog.pg_constraint AS con")
	assert.Contains(t, sql, "con.contype = 'f'")
	assert.Contains(t, sql, "child_cls.oid = con.conrelid")
	assert.Contains(t, sql, "parent_cls.oid = con.confrelid")
	assert.Contains(t, sql, "child_ns.nspname AS child_schema")
	assert.Contains(t, sql, "parent_ns.nspname AS parent_schema")
	assert.Contains(t, sql, "child_ns.nspname <> 'pg_catalog'")
	assert.Contains(t, sql, "parent_ns.nspname <> 'pg_catalog'")
	assert.Contains(t, sql, "child_ns.nspname NOT LIKE 'pg_%'")
	assert.Contains(t, sql, "parent_ns.nspname NOT LIKE 'pg_%'")
}

func TestListSequencesSQLUsesDependClassAndAttributeOwnership(t *testing.T) {
	t.Parallel()
	sql := compactSQL(pgschema.ListSequencesSQL())

	assert.Contains(t, sql, "FROM pg_catalog.pg_class AS seq_cls")
	assert.Contains(t, sql, "JOIN pg_catalog.pg_depend AS dep")
	assert.Contains(t, sql, "JOIN pg_catalog.pg_attribute AS attr")
	assert.Contains(t, sql, "seq_cls.relkind = 'S'")
	assert.Contains(t, sql, "dep.classid = 'pg_class'::regclass")
	assert.Contains(t, sql, "dep.refclassid = 'pg_class'::regclass")
	assert.Contains(t, sql, "dep.deptype IN ('a', 'i')")
	assert.Contains(t, sql, "seq_ns.nspname <> 'pg_catalog'")
	assert.Contains(t, sql, "table_ns.nspname <> 'pg_catalog'")
	assert.Contains(t, sql, "attr.attnum = dep.refobjsubid")
	assert.Contains(t, sql, "NOT attr.attisdropped")
}

func compactSQL(sql string) string {
	return strings.Join(strings.Fields(sql), " ")
}

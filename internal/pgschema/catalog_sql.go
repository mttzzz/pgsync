package pgschema

const listDatabasesBaseSQL = `
SELECT
    d.datname AS name,
    pg_catalog.pg_database_size(d.datname) AS size_bytes,
    pg_catalog.pg_get_userbyid(d.datdba) AS owner
FROM pg_catalog.pg_database AS d
WHERE d.datallowconn
  AND NOT d.datistemplate`

const listDatabasesOrderSQL = `
ORDER BY d.datname`

const listTablesSQL = `
SELECT
    n.nspname AS schema_name,
    c.relname AS table_name,
    pg_catalog.pg_total_relation_size(c.oid) AS size_bytes,
    GREATEST(c.reltuples, 0)::bigint AS estimated_rows
FROM pg_catalog.pg_class AS c
JOIN pg_catalog.pg_namespace AS n ON n.oid = c.relnamespace
WHERE c.relkind IN ('r', 'p')
  AND n.nspname <> 'pg_catalog'
  AND n.nspname <> 'information_schema'
  AND n.nspname NOT LIKE 'pg_%'
ORDER BY n.nspname, c.relname`

const listFKDepsSQL = `
SELECT
    child_ns.nspname AS child_schema,
    child_cls.relname AS child_table,
    parent_ns.nspname AS parent_schema,
    parent_cls.relname AS parent_table
FROM pg_catalog.pg_constraint AS con
JOIN pg_catalog.pg_class AS child_cls ON child_cls.oid = con.conrelid
JOIN pg_catalog.pg_namespace AS child_ns ON child_ns.oid = child_cls.relnamespace
JOIN pg_catalog.pg_class AS parent_cls ON parent_cls.oid = con.confrelid
JOIN pg_catalog.pg_namespace AS parent_ns ON parent_ns.oid = parent_cls.relnamespace
WHERE con.contype = 'f'
  AND child_cls.relkind IN ('r', 'p')
  AND parent_cls.relkind IN ('r', 'p')
  AND child_ns.nspname <> 'pg_catalog'
  AND parent_ns.nspname <> 'pg_catalog'
  AND child_ns.nspname <> 'information_schema'
  AND parent_ns.nspname <> 'information_schema'
  AND child_ns.nspname NOT LIKE 'pg_%'
  AND parent_ns.nspname NOT LIKE 'pg_%'
ORDER BY child_ns.nspname, child_cls.relname, parent_ns.nspname, parent_cls.relname`

const listSequencesSQL = `
SELECT
    seq_ns.nspname AS sequence_schema,
    seq_cls.relname AS sequence_name,
    table_ns.nspname AS table_schema,
    table_cls.relname AS table_name,
    attr.attname AS column_name
FROM pg_catalog.pg_class AS seq_cls
JOIN pg_catalog.pg_namespace AS seq_ns ON seq_ns.oid = seq_cls.relnamespace
JOIN pg_catalog.pg_depend AS dep ON dep.objid = seq_cls.oid
JOIN pg_catalog.pg_class AS table_cls ON table_cls.oid = dep.refobjid
JOIN pg_catalog.pg_namespace AS table_ns ON table_ns.oid = table_cls.relnamespace
JOIN pg_catalog.pg_attribute AS attr ON attr.attrelid = table_cls.oid
    AND attr.attnum = dep.refobjsubid
WHERE seq_cls.relkind = 'S'
  AND dep.classid = 'pg_class'::regclass
  AND dep.refclassid = 'pg_class'::regclass
  AND dep.deptype IN ('a', 'i')
  AND table_cls.relkind IN ('r', 'p')
  AND attr.attnum > 0
  AND NOT attr.attisdropped
  AND seq_ns.nspname <> 'pg_catalog'
  AND table_ns.nspname <> 'pg_catalog'
  AND seq_ns.nspname <> 'information_schema'
  AND table_ns.nspname <> 'information_schema'
  AND seq_ns.nspname NOT LIKE 'pg_%'
  AND table_ns.nspname NOT LIKE 'pg_%'
ORDER BY seq_ns.nspname, seq_cls.relname`

/* ListDatabasesSQL returns the query used to list user databases. */
func ListDatabasesSQL() string {
	return listDatabasesBaseSQL + `
  AND d.datname <> 'postgres'` + listDatabasesOrderSQL
}

/* ListDatabasesIncludingSystemSQL returns the query used to list non-template databases, including system databases. */
func ListDatabasesIncludingSystemSQL() string {
	return listDatabasesBaseSQL + listDatabasesOrderSQL
}

/* ListTablesSQL returns the query used to list ordinary and partitioned user tables. */
func ListTablesSQL() string {
	return listTablesSQL
}

/* ListFKDepsSQL returns the query used to list foreign-key dependencies between user tables. */
func ListFKDepsSQL() string {
	return listFKDepsSQL
}

/* ListSequencesSQL returns the query used to list owned sequences for user table columns. */
func ListSequencesSQL() string {
	return listSequencesSQL
}

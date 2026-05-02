package models

import "fmt"

/* Table describes a PostgreSQL table. */
type Table struct {
	Schema    string
	Name      string
	SizeBytes int64
	Rows      int64
}

/* QualifiedName returns the SQL-quoted "schema"."name". */
func (t Table) QualifiedName() string {
	return fmt.Sprintf(`"%s"."%s"`, t.Schema, t.Name)
}

/* FKDep represents a foreign-key edge: From depends on To. */
type FKDep struct {
	From Table
	To   Table
}

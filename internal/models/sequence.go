package models

import "fmt"

/* Sequence describes a PostgreSQL sequence and the table column it owns. */
type Sequence struct {
	Schema      string
	Name        string
	TableSchema string
	TableName   string
	ColumnName  string
}

/* QualifiedName returns the SQL-quoted "schema"."name". */
func (s Sequence) QualifiedName() string {
	return fmt.Sprintf(`"%s"."%s"`, s.Schema, s.Name)
}

/* OwnedTable returns the table owned by this sequence. */
func (s Sequence) OwnedTable() Table {
	return Table{Schema: s.TableSchema, Name: s.TableName}
}

package pgdb

import (
	"errors"
	"fmt"
	"strings"

	"github.com/mttzzz/pgsync/internal/models"
)

/* QuoteIdent quotes one PostgreSQL identifier and doubles embedded quotes. */
func QuoteIdent(s string) (string, error) {
	if strings.TrimSpace(s) == "" {
		return "", errors.New("identifier is required")
	}
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`, nil
}

/* QuoteQualified quotes a schema-qualified PostgreSQL table name. */
func QuoteQualified(t models.Table) (string, error) {
	schema, err := QuoteIdent(t.Schema)
	if err != nil {
		return "", fmt.Errorf("quote schema: %w", err)
	}
	name, err := QuoteIdent(t.Name)
	if err != nil {
		return "", fmt.Errorf("quote table: %w", err)
	}
	return schema + "." + name, nil
}

/* ParseTableName parses table or schema.table text using defaultSchema for
 * unqualified names.
 */
func ParseTableName(raw string, defaultSchema string) (models.Table, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return models.Table{}, errors.New("table name is required")
	}

	parts := strings.Split(trimmed, ".")
	if len(parts) > 2 {
		return models.Table{}, fmt.Errorf("table name must be name or schema.name, got %q", raw)
	}

	schema := strings.TrimSpace(defaultSchema)
	name := strings.TrimSpace(parts[0])
	if len(parts) == 2 {
		schema = strings.TrimSpace(parts[0])
		name = strings.TrimSpace(parts[1])
	}
	if schema == "" {
		return models.Table{}, errors.New("table schema is required")
	}
	if name == "" {
		return models.Table{}, errors.New("table name is required")
	}
	return models.Table{Schema: schema, Name: name}, nil
}

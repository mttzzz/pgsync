package models_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/mttzzz/pgsync/internal/models"
)

func TestSequenceQualifiedName(t *testing.T) {
	t.Parallel()
	sequence := models.Sequence{Schema: "public", Name: "users_id_seq"}
	assert.Equal(t, `"public"."users_id_seq"`, sequence.QualifiedName())
}

func TestSequenceOwnedTable(t *testing.T) {
	t.Parallel()
	sequence := models.Sequence{TableSchema: "app", TableName: "orders", ColumnName: "id"}
	assert.Equal(t, models.Table{Schema: "app", Name: "orders"}, sequence.OwnedTable())
}

func TestSequenceEmptyFields(t *testing.T) {
	t.Parallel()
	sequence := models.Sequence{}
	assert.Equal(t, `"".""`, sequence.QualifiedName())
	assert.Equal(t, models.Table{}, sequence.OwnedTable())
}

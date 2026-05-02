package models_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/mttzzz/pgsync/internal/models"
)

func TestDatabaseString(t *testing.T) {
	t.Parallel()
	database := models.Database{Name: "ai", SizeBytes: 1024 * 1024}
	assert.Contains(t, database.String(), "ai")
	assert.Contains(t, database.String(), "1.0 MB")
}

func TestTableQualifiedName(t *testing.T) {
	t.Parallel()
	table := models.Table{Schema: "public", Name: "users"}
	assert.Equal(t, `"public"."users"`, table.QualifiedName())
}

func TestSyncPlanIsEmpty(t *testing.T) {
	t.Parallel()
	assert.True(t, (&models.SyncPlan{}).IsEmpty())
	assert.False(t, (&models.SyncPlan{Database: "x"}).IsEmpty())
}

func TestProgressPercent(t *testing.T) {
	t.Parallel()
	progress := models.Progress{Done: 25, Total: 100}
	assert.InDelta(t, 25.0, progress.Percent(), 0.001)

	zero := models.Progress{Done: 1, Total: 0}
	assert.Equal(t, 0.0, zero.Percent())
}

func TestSyncResultDuration(t *testing.T) {
	t.Parallel()
	start := time.Now()
	result := models.SyncResult{StartedAt: start, FinishedAt: start.Add(2 * time.Second)}
	assert.Equal(t, 2*time.Second, result.Duration())
}

func TestFormatBytes(t *testing.T) {
	t.Parallel()
	cases := map[int64]string{
		0:                         "0 B",
		512:                       "512 B",
		1024:                      "1.0 KB",
		1024 * 1024:               "1.0 MB",
		1024 * 1024 * 5:           "5.0 MB",
		1024 * 1024 * 1024:        "1.0 GB",
		1024 * 1024 * 1024 * 1024: "1.0 TB",
	}
	for in, want := range cases {
		assert.Equal(t, want, models.FormatBytes(in), "in=%d", in)
	}
}

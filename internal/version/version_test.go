package version_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/mttzzz/pgsync/internal/version"
)

func TestStringContainsAllFields(t *testing.T) {
	t.Parallel()
	got := version.String()
	assert.Contains(t, got, version.Version)
	assert.Contains(t, got, version.GitCommit)
	assert.Contains(t, got, version.BuildDate)
}

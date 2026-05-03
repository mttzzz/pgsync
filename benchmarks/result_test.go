package benchmarks

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResultRoundtripAndWrite(t *testing.T) {
	t.Parallel()
	result := Result{SchemaVersion: 1, Fixture: "tiny", Engine: "native", Threads: 4, DurationMS: 100, Throughput: Throughput{RowsPerSec: 10, BytesPerSec: 20}}
	data, err := json.Marshal(result)
	require.NoError(t, err)
	assert.Contains(t, string(data), "schema_version")
	dir := t.TempDir()
	require.NoError(t, WriteResult(dir, result))
	assert.FileExists(t, filepath.Join(dir, "tiny.json"))
}

func TestHarnessSelection(t *testing.T) {
	t.Parallel()
	opts := HarnessOptions{Fixtures: []string{"tiny"}}
	assert.True(t, opts.Selected("tiny"))
	assert.False(t, opts.Selected("large"))
}

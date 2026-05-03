package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateDeterministicTinyFixture(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	first := filepath.Join(dir, "first.sql.gz")
	second := filepath.Join(dir, "second.sql.gz")
	meta, err := Generate(Options{Size: "tiny", Seed: 42, Out: first})
	require.NoError(t, err)
	_, err = Generate(Options{Size: "tiny", Seed: 42, Out: second})
	require.NoError(t, err)
	firstBytes, err := os.ReadFile(first) // #nosec G304 -- test reads a temp file created by this test.
	require.NoError(t, err)
	secondBytes, err := os.ReadFile(second) // #nosec G304 -- test reads a temp file created by this test.
	require.NoError(t, err)
	assert.Equal(t, firstBytes, secondBytes)
	assert.Equal(t, 3, meta.ExpectedTableCount)
	assert.FileExists(t, first+".json")
}

func TestGenerateRejectsBadInput(t *testing.T) {
	t.Parallel()
	_, err := Generate(Options{Size: "huge", Seed: 1, Out: filepath.Join(t.TempDir(), "x.sql.gz")})
	require.Error(t, err)
	_, err = Generate(Options{Size: "tiny", Seed: 1})
	require.Error(t, err)
}

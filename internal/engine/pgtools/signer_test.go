package pgtools

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDarwinSignerTreatsAlreadySignedAsSuccess(t *testing.T) {
	t.Parallel()
	signer := NewSigner("darwin", fakeSignRunner{stderr: []byte("/tmp/pg_dump: is already signed"), err: errors.New("exit 1")}, nil)

	err := signer.Sign(context.Background(), "/tmp/pg_dump")

	require.NoError(t, err)
	assert.True(t, signer.ShouldSign("darwin-arm64", "bin/pg_dump"))
	assert.False(t, signer.ShouldSign("linux-amd64", "bin/pg_dump"))
}

type fakeSignRunner struct {
	stderr []byte
	err    error
}

func (f fakeSignRunner) Run(context.Context, string, []string, []string) ([]byte, []byte, error) {
	return nil, f.stderr, f.err
}

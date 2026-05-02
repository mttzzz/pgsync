package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOptionsValidate(t *testing.T) {
	t.Parallel()
	assert.NoError(t, (Options{Output: "text", Engine: "auto"}).Validate())
	assert.NoError(t, (Options{Output: "json", Engine: ""}).Validate())
	assert.Error(t, (Options{Output: "yaml"}).Validate())
	assert.Error(t, (Options{Output: "text", Engine: "bad"}).Validate())
	assert.Error(t, (Options{Output: "text", Threads: -1}).Validate())
}

func TestValidateModes(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"text", "json"} {
		assert.NoError(t, ValidateOutputMode(mode))
	}
	for _, mode := range []string{"auto", "native", "external"} {
		assert.NoError(t, ValidateEngineMode(mode))
	}
	assert.Error(t, ValidateOutputMode(""))
	assert.Error(t, ValidateEngineMode(""))
}

func TestOptionsLogLevel(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "error", Options{Quiet: true, Verbose: true}.LogLevel("info"))
	assert.Equal(t, "debug", Options{Verbose: true}.LogLevel("info"))
	assert.Equal(t, "warn", Options{}.LogLevel("warn"))
	assert.Equal(t, "info", Options{}.LogLevel(""))
	assert.Equal(t, "", Options{}.ConfigPath)
	assert.ErrorIs(t, ErrInvalidOptions, ErrInvalidOptions)
}

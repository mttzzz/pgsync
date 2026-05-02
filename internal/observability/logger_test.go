package observability_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mttzzz/pgsync/internal/observability"
)

func TestNewLoggerText(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	log, err := observability.NewLogger(observability.Options{
		Level: "info", Format: "text", Out: &buf,
	})
	require.NoError(t, err)
	log.Info("hello", "key", "val")
	out := buf.String()
	assert.Contains(t, out, "hello")
	assert.Contains(t, out, "key=val")
}

func TestNewLoggerJSON(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	log, err := observability.NewLogger(observability.Options{
		Level: "debug", Format: "json", Out: &buf,
	})
	require.NoError(t, err)
	log.Debug("event", "n", 1)
	line := strings.TrimSpace(buf.String())
	var got map[string]any
	require.NoError(t, json.Unmarshal([]byte(line), &got))
	assert.Equal(t, "event", got["msg"])
	assert.EqualValues(t, 1, got["n"])
	assert.Equal(t, "DEBUG", got["level"])
}

func TestNewLoggerWarnErrorAndNilOut(t *testing.T) {
	t.Parallel()
	warnLog, err := observability.NewLogger(observability.Options{Level: "warn", Format: "text"})
	require.NoError(t, err)
	assert.NotNil(t, warnLog)
	errorLog, err := observability.NewLogger(observability.Options{Level: "error", Format: "json"})
	require.NoError(t, err)
	assert.NotNil(t, errorLog)
}

func TestNewLoggerInvalidLevel(t *testing.T) {
	t.Parallel()
	_, err := observability.NewLogger(observability.Options{Level: "bogus", Format: "text"})
	require.Error(t, err)
}

func TestNewLoggerInvalidFormat(t *testing.T) {
	t.Parallel()
	_, err := observability.NewLogger(observability.Options{Level: "info", Format: "yaml"})
	require.Error(t, err)
}

func TestNewLoggerLevelFiltersDebug(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	log, err := observability.NewLogger(observability.Options{Level: "info", Format: "text", Out: &buf})
	require.NoError(t, err)
	log.Debug("filtered")
	assert.Empty(t, buf.String())
}

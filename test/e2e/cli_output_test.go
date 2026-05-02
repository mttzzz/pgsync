package e2e_test

import (
	"encoding/json"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCLIHelpVersionAndJSONDoctor(t *testing.T) {
	t.Parallel()
	help := runPGSync(t, "--help")
	assert.Contains(t, help, "PostgreSQL database sync")

	version := runPGSync(t, "version")
	assert.Contains(t, version, "pgsync")

	doctor := runPGSync(t, "--output=json", "doctor")
	var payload map[string]string
	require.NoError(t, json.Unmarshal([]byte(doctor), &payload))
	assert.Equal(t, "doctor.done", payload["event"])
}

func runPGSync(t *testing.T, args ...string) string {
	t.Helper()
	cmdArgs := append([]string{"run", "./cmd/pgsync"}, args...)
	cmd := exec.Command("go", cmdArgs...) // #nosec G204 -- test executes the local Go tool with controlled args.
	cmd.Dir = "../.."
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
	return string(out)
}

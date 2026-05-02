//go:build integration

// Package integration contains Docker-backed integration tests for pgsync.
package integration

import (
	"strings"
	"testing"

	"github.com/mttzzz/pgsync/internal/cli"
	"github.com/mttzzz/pgsync/internal/engine"
	"github.com/mttzzz/pgsync/test/helpers"
)

func TestCLISyncJSONModeEmitsLifecycleEvents(t *testing.T) {
	skipIfSystemPgDumpUnavailable(t)
	ctx, cancel := integrationContext(t)
	defer cancel()

	source, target := startSyncContainers(ctx, t)
	loadTinyFixture(ctx, t, source)
	configPath := writeCLIConfig(t, source, target)

	stdout, stderr, err := runCLI(ctx, t,
		"--config", configPath,
		"--output=json",
		"sync", tinyDatabaseName,
		"--yes",
		"--threads=2",
		"--use-system-pgtools",
	)
	if err != nil {
		t.Fatalf("run CLI sync JSON mode: exit=%d err=%v stderr=%q", cli.ExitCode(err), err, stderr)
	}
	if code := cli.ExitCode(err); code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	assertNoSecretInStderr(t, stderr, source, target)
	if stderr != "" {
		t.Fatalf("expected empty stderr on successful JSON sync, got %q", stderr)
	}

	records := decodeNDJSON(t, stdout)
	assertEventsContainInOrder(t, ndjsonEventNames(records), []string{
		engine.EventSyncStart,
		engine.EventSchemaPreDataStart,
		engine.EventTableCopyStart,
		engine.EventTableCopyDone,
		engine.EventSchemaPostDataDone,
		engine.EventSyncDone,
	})
	helpers.AssertTableRowCountsEqual(ctx, t, source, target, tinySyncTables)
}

func assertNoSecretInStderr(
	t testing.TB,
	stderr string,
	source helpers.PostgresContainer,
	target helpers.PostgresContainer,
) {
	t.Helper()
	for _, secret := range []string{source.Password, target.Password} {
		if secret != "" && strings.Contains(stderr, secret) {
			t.Fatalf("stderr leaked secret %q: %q", secret, stderr)
		}
	}
}

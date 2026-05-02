//go:build integration

// Package integration contains Docker-backed integration tests for pgsync.
package integration

import (
	"context"
	"strings"
	"testing"

	"github.com/mttzzz/pgsync/test/helpers"
)

const dryRunSentinelValue = "target must survive dry run"

func TestCLIDryRunPrintsPlanAndDoesNotMutateTarget(t *testing.T) {
	ctx, cancel := integrationContext(t)
	defer cancel()

	source, target := startSyncContainers(ctx, t)
	loadTinyFixture(ctx, t, source)
	createDryRunSentinel(ctx, t, target)
	before := sentinelValue(ctx, t, target)
	configPath := writeCLIConfig(t, source, target)

	stdout, stderr, err := runCLI(ctx, t,
		"--config", configPath,
		"--no-color",
		"sync", tinyDatabaseName,
		"--dry-run",
	)
	if err != nil {
		t.Fatalf("run CLI dry-run: %v stderr=%q", err, stderr)
	}
	if stderr != "" {
		t.Fatalf("expected empty stderr for dry-run, got %q", stderr)
	}
	if before != dryRunSentinelValue {
		t.Fatalf("test setup sentinel mismatch: got %q", before)
	}
	if after := sentinelValue(ctx, t, target); after != before {
		t.Fatalf("dry-run mutated target sentinel: before=%q after=%q", before, after)
	}
	assertDryRunPlanOutput(t, stdout)
}

func createDryRunSentinel(ctx context.Context, t testing.TB, target helpers.PostgresContainer) {
	t.Helper()
	execSQL(ctx, t, target, `
CREATE TABLE public.dry_run_sentinel (
    id integer PRIMARY KEY,
    value text NOT NULL
);
INSERT INTO public.dry_run_sentinel (id, value) VALUES (1, 'target must survive dry run');`)
}

func assertDryRunPlanOutput(t testing.TB, output string) {
	t.Helper()
	for _, expected := range []string{"plan database=tiny", "tables=3", "public.users", "public.orders", "public.order_items"} {
		if !strings.Contains(output, expected) {
			t.Fatalf("expected dry-run output to contain %q, got %q", expected, output)
		}
	}
}

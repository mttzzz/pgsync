//go:build integration

// Package integration contains Docker-backed integration tests for pgsync.
package integration

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/mttzzz/pgsync/test/helpers"
)

const harnessTimeout = 3 * time.Minute

var tinyFixtureTables = []string{
	"public.users",
	"public.orders",
	"public.order_items",
}

func TestHarnessLoadsFixturesAndAssertions(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), harnessTimeout)
	defer cancel()

	source := helpers.StartPostgres(ctx, t, "source_harness")
	target := helpers.StartPostgres(ctx, t, "target_harness")

	tinyPath := fixturePath("tiny.sql")
	helpers.ExecSQLFile(ctx, t, source, tinyPath)
	helpers.ExecSQLFile(ctx, t, target, tinyPath)
	helpers.ExecSQLFile(ctx, t, source, fixturePath("partial.sql"))

	helpers.AssertTableRowCountsEqual(ctx, t, source, target, tinyFixtureTables)
	helpers.AssertTableChecksumsEqual(ctx, t, source, target, tinyFixtureTables)
	helpers.AssertSequencesUsable(ctx, t, target, tinyFixtureTables)
	helpers.AssertIndexExists(ctx, t, target, "users_name_idx")
	helpers.AssertIndexExists(ctx, t, target, "order_items_order_sku_idx")
	helpers.AssertFKExists(ctx, t, target, "orders_user_id_fkey")
	helpers.AssertFKExists(ctx, t, target, "order_items_order_id_fkey")
}

func fixturePath(name string) string {
	return filepath.Join("fixtures", name)
}

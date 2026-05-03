package benchmarks

import "testing"

func BenchmarkSyncTiny(b *testing.B)   { benchmarkSelectedFixture(b, "tiny") }
func BenchmarkSyncSmall(b *testing.B)  { benchmarkSelectedFixture(b, "small") }
func BenchmarkSyncMedium(b *testing.B) { benchmarkSelectedFixture(b, "medium") }
func BenchmarkSyncLarge(b *testing.B)  { benchmarkSelectedFixture(b, "large") }

func benchmarkSelectedFixture(b *testing.B, fixture string) {
	opts := HarnessOptionsFromEnv()
	if !opts.Selected(fixture) {
		b.Skipf("fixture %s not selected", fixture)
	}
	for b.Loop() {
		_ = Result{SchemaVersion: 1, Fixture: fixture, Engine: opts.Engine, Threads: opts.Threads, DurationMS: 1, Host: CurrentHost()}
	}
}

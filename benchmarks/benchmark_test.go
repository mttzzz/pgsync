package benchmarks

import "testing"

func BenchmarkPlannerPlaceholder(b *testing.B) {
	for b.Loop() {
		_ = "pgsync"
	}
}

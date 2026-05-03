// Package benchmarks contains benchmark harnesses and result helpers.
package benchmarks

import "testing"

func BenchmarkPlannerPlaceholder(b *testing.B) {
	for b.Loop() {
		_ = "pgsync"
	}
}

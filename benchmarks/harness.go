package benchmarks

import (
	"os"
	"runtime"
	"strings"
)

// HarnessOptions configures the benchmark harness from environment variables.
type HarnessOptions struct {
	Fixtures []string
	Threads  int
	Engine   string
	OutDir   string
}

// HarnessOptionsFromEnv returns benchmark harness options from PGSYNC_BENCH_* variables.
func HarnessOptionsFromEnv() HarnessOptions {
	fixtures := splitEnv("PGSYNC_BENCH_FIXTURES", "tiny,small")
	engine := os.Getenv("PGSYNC_BENCH_ENGINE")
	if engine == "" {
		engine = "native"
	}
	return HarnessOptions{Fixtures: fixtures, Threads: runtime.NumCPU(), Engine: engine, OutDir: os.Getenv("PGSYNC_BENCH_OUT")}
}

func splitEnv(key, fallback string) []string {
	value := os.Getenv(key)
	if value == "" {
		value = fallback
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

// Selected reports whether fixture should run in this benchmark invocation.
func (o HarnessOptions) Selected(fixture string) bool {
	for _, selected := range o.Fixtures {
		if selected == fixture {
			return true
		}
	}
	return false
}

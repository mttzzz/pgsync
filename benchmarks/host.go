package benchmarks

import "runtime"

// CurrentHost returns best-effort host metadata for benchmark results.
func CurrentHost() Host {
	return Host{OS: runtime.GOOS, Arch: runtime.GOARCH, Cores: runtime.NumCPU()}
}

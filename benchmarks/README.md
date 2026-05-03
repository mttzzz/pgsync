# Benchmarks

`make bench` runs the default tiny/small benchmark selection. Real container-backed sync benchmarks can write JSON results through `WriteResult` into `benchmarks/results/<sha>/`.

Compare results with:

`go run ./benchmarks/compare.go --baseline benchmarks/results/main --candidate benchmarks/results/<sha> --threshold 0.15`

Promote baselines only after reviewing stable CI or release-hardware runs.

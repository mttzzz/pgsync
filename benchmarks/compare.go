//go:build ignore

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

type result struct {
	Fixture    string `json:"fixture"`
	DurationMS int64  `json:"duration_ms"`
	Throughput struct {
		RowsPerSec  float64 `json:"rows_per_sec"`
		BytesPerSec float64 `json:"bytes_per_sec"`
	} `json:"throughput"`
}

func main() {
	baseline := flag.String("baseline", "benchmarks/results/main", "baseline result directory")
	candidate := flag.String("candidate", "benchmarks/results/local", "candidate result directory")
	threshold := flag.Float64("threshold", 0.15, "allowed regression ratio")
	flag.Parse()
	code, err := compareDirs(*baseline, *candidate, *threshold)
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(code)
	}
	os.Exit(code)
}

func compareDirs(baselineDir, candidateDir string, threshold float64) (int, error) {
	entries, err := os.ReadDir(baselineDir)
	if err != nil {
		return 2, fmt.Errorf("read baseline dir: %w", err)
	}
	regressed := false
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		base, err := readResult(filepath.Join(baselineDir, entry.Name()))
		if err != nil {
			return 2, err
		}
		cand, err := readResult(filepath.Join(candidateDir, entry.Name()))
		if err != nil {
			return 2, err
		}
		fixture := base.Fixture
		if fixture == "" {
			fixture = entry.Name()
		}
		if cand.DurationMS > int64(float64(base.DurationMS)*(1+threshold)) {
			fmt.Printf("REGRESSION duration %s: baseline=%d candidate=%d\n", fixture, base.DurationMS, cand.DurationMS)
			regressed = true
		}
		if cand.Throughput.RowsPerSec < base.Throughput.RowsPerSec*(1-threshold) {
			fmt.Printf("REGRESSION rows/s %s: baseline=%.2f candidate=%.2f\n", fixture, base.Throughput.RowsPerSec, cand.Throughput.RowsPerSec)
			regressed = true
		}
		if cand.Throughput.BytesPerSec < base.Throughput.BytesPerSec*(1-threshold) {
			fmt.Printf("REGRESSION bytes/s %s: baseline=%.2f candidate=%.2f\n", fixture, base.Throughput.BytesPerSec, cand.Throughput.BytesPerSec)
			regressed = true
		}
	}
	if regressed {
		return 1, nil
	}
	fmt.Println("benchmark comparison passed")
	return 0, nil
}

func readResult(path string) (result, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return result{}, fmt.Errorf("read benchmark result %s: %w", path, err)
	}
	var out result
	if err := json.Unmarshal(data, &out); err != nil {
		return result{}, fmt.Errorf("decode benchmark result %s: %w", path, err)
	}
	return out, nil
}

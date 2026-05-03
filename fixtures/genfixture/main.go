package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	size := flag.String("size", "tiny", "fixture size: tiny, medium, large")
	seed := flag.Int64("seed", 42, "deterministic random seed")
	out := flag.String("out", "fixtures/tiny.sql.gz", "output .sql.gz path")
	flag.Parse()
	metadata, err := Generate(Options{Size: *size, Seed: *seed, Out: *out})
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	_, _ = fmt.Fprintf(os.Stdout, "generated %s fixture at %s (%d tables)\n", metadata.Size, *out, metadata.ExpectedTableCount)
}

/*
 * Phase 1 entrypoint: prints version. Real CLI surface lands in Phase 2.
 */
package main

import (
	"fmt"
	"os"

	"github.com/mttzzz/pgsync/internal/version"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "version" {
		fmt.Println(version.String())
		return
	}
	fmt.Println("pgsync — Phase 1 foundation. CLI commands ship in Phase 2.")
	fmt.Println(version.String())
}

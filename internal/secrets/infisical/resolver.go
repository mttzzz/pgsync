/* Package infisical resolves a project's database name from its local
 * Infisical workspace, by locating the nearest .infisical.json and
 * shelling out to `infisical export --env=dev --format=dotenv --silent`.
 */
package infisical

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

/* Resolver looks up the DB name for the project rooted at CWD.
 * LookPath and Run are injectable for tests; nil means use real os/exec. */
type Resolver struct {
	CWD      string
	LookPath func(name string) (string, error)
	Run      func(ctx context.Context, dir, name string, args ...string) ([]byte, []byte, error)
}

/* ResolveDBName walks up from r.CWD until it finds a .infisical.json,
 * then runs `infisical export --env=dev --format=dotenv --silent` from
 * that directory and extracts the DB name from POSTGRES_URL or DB_DATABASE. */
func (r Resolver) ResolveDBName(ctx context.Context) (string, error) {
	root, err := findInfisicalRoot(r.CWD)
	if err != nil {
		return "", err
	}
	lookPath := r.LookPath
	if lookPath == nil {
		lookPath = exec.LookPath
	}
	if _, err := lookPath("infisical"); err != nil {
		return "", fmt.Errorf("pgsync: 'infisical' CLI not found in PATH (install: https://infisical.com/docs/cli/overview)")
	}
	_ = root
	return "", fmt.Errorf("not implemented")
}

func findInfisicalRoot(start string) (string, error) {
	dir := start
	if dir == "" {
		var err error
		dir, err = os.Getwd()
		if err != nil {
			return "", fmt.Errorf("getwd: %w", err)
		}
	}
	dir, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("abs: %w", err)
	}
	for {
		if _, statErr := os.Stat(filepath.Join(dir, ".infisical.json")); statErr == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("pgsync: no .infisical.json found walking up from %s", start)
		}
		dir = parent
	}
}

/* Package infisical resolves a project's database name from its local
 * Infisical workspace, by locating the nearest .infisical.json and
 * shelling out to `infisical export --env=dev --format=dotenv --silent`.
 */
package infisical

import (
	"bytes"
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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
	run := r.Run
	if run == nil {
		run = defaultRun
	}
	stdout, stderr, err := run(ctx, root, "infisical", "export", "--env=dev", "--format=dotenv", "--silent")
	if err != nil {
		return "", fmt.Errorf("pgsync: infisical export failed: %s: %w", strings.TrimSpace(string(stderr)), err)
	}
	env := parseDotenv(stdout)
	if u := strings.TrimSpace(env["POSTGRES_URL"]); u != "" {
		name, parseErr := dbFromPostgresURL(u)
		if parseErr != nil {
			return "", fmt.Errorf("pgsync: parse POSTGRES_URL from Infisical: %w", parseErr)
		}
		if name != "" {
			return name, nil
		}
	}
	if name := strings.TrimSpace(env["DB_DATABASE"]); name != "" {
		return name, nil
	}
	return "", fmt.Errorf("pgsync: cannot resolve DB name from Infisical (env=dev): neither POSTGRES_URL nor DB_DATABASE is set")
}

func defaultRun(ctx context.Context, dir, name string, args ...string) ([]byte, []byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.Bytes(), stderr.Bytes(), err
}

func parseDotenv(blob []byte) map[string]string {
	out := make(map[string]string)
	for _, line := range strings.Split(string(blob), "\n") {
		k, v, ok := parseDotenvLine(line)
		if ok {
			out[k] = v
		}
	}
	return out
}

func parseDotenvLine(line string) (string, string, bool) {
	line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
	if line == "" || strings.HasPrefix(line, "#") {
		return "", "", false
	}
	line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
	key, value, ok := strings.Cut(line, "=")
	if !ok {
		return "", "", false
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return "", "", false
	}
	value = strings.TrimSpace(value)
	if len(value) >= 2 {
		q := value[0]
		if (q == '\'' || q == '"') && value[len(value)-1] == q {
			return key, value[1 : len(value)-1], true
		}
	}
	if before, _, hashOK := strings.Cut(value, " #"); hashOK {
		value = strings.TrimSpace(before)
	}
	return key, value, true
}

func dbFromPostgresURL(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("parse url: %w", err)
	}
	if u.Scheme != "postgres" && u.Scheme != "postgresql" {
		return "", fmt.Errorf("scheme must be postgres or postgresql, got %q", u.Scheme)
	}
	return strings.TrimPrefix(u.Path, "/"), nil
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

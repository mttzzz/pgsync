package native

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/mttzzz/pgsync/internal/config"
	"github.com/mttzzz/pgsync/internal/pgdb"
)

var createExtensionRE = regexp.MustCompile(`(?is)\bCREATE\s+EXTENSION\s+(?:IF\s+NOT\s+EXISTS\s+)?("(?:[^"]|"")*"|[A-Za-z_][A-Za-z0-9_$]*)`)

// ExtensionChecker verifies that the target PostgreSQL server has source-required extensions installed.
type ExtensionChecker struct {
	Connector pgdb.Connector
}

// CheckPreData verifies CREATE EXTENSION statements before the target database is reset.
func (c *ExtensionChecker) CheckPreData(ctx context.Context, local config.Connection, preDataSQL string) (err error) {
	if c == nil {
		return errors.New("extension checker is required")
	}
	if c.Connector == nil {
		return errors.New("extension checker connector is required")
	}
	required := schemaExtensions(preDataSQL)
	if len(required) == 0 {
		return nil
	}
	endpoint := pgdb.EndpointFromConfig(local, maintenanceDatabase(local))
	conn, err := c.Connector.Connect(ctx, endpoint)
	if err != nil {
		return fmt.Errorf("connect target maintenance database %q for extension preflight: %w", endpoint.Database, err)
	}
	defer func() {
		if closeErr := conn.Close(context.WithoutCancel(ctx)); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close target extension preflight connection: %w", closeErr))
		}
	}()
	available, err := availableExtensions(ctx, conn, required)
	if err != nil {
		return err
	}
	missing := missingExtensions(required, available)
	if len(missing) > 0 {
		return fmt.Errorf(
			"missing local PostgreSQL extensions before target reset: %s; install them on the target PostgreSQL server and retry",
			strings.Join(missing, ", "),
		)
	}
	return nil
}

func schemaExtensions(sql string) []string {
	matches := createExtensionRE.FindAllStringSubmatch(sql, -1)
	seen := map[string]struct{}{}
	out := make([]string, 0, len(matches))
	for _, match := range matches {
		name := normalizeExtensionName(match[1])
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	slices.Sort(out)
	return out
}

func normalizeExtensionName(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	if strings.HasPrefix(trimmed, `"`) && strings.HasSuffix(trimmed, `"`) && len(trimmed) >= 2 {
		return strings.ReplaceAll(trimmed[1:len(trimmed)-1], `""`, `"`)
	}
	return strings.ToLower(trimmed)
}

func availableExtensions(ctx context.Context, conn pgdb.CopyConn, required []string) (map[string]struct{}, error) {
	rows, err := conn.Query(ctx, "SELECT name FROM pg_available_extensions WHERE name::text = ANY($1::text[])", required)
	if err != nil {
		return nil, fmt.Errorf("query target available extensions: %w", err)
	}
	defer rows.Close()
	available := map[string]struct{}{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("scan target available extension: %w", err)
		}
		available[name] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read target available extensions: %w", err)
	}
	return available, nil
}

func missingExtensions(required []string, available map[string]struct{}) []string {
	missing := make([]string, 0, len(required))
	for _, name := range required {
		if _, ok := available[name]; !ok {
			missing = append(missing, name)
		}
	}
	return missing
}

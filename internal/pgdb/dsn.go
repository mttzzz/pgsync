// Package pgdb contains PostgreSQL connection and SQL identifier primitives.
package pgdb

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/mttzzz/pgsync/internal/config"
)

const (
	defaultMaintenanceDatabase = "postgres"
	maskedPassword             = "xxxxx"
)

var passwordAssignmentPattern = regexp.MustCompile(`(?i)(password=)([^\s]+)`)

/* Endpoint is a PostgreSQL connection endpoint after config and CLI database
 * overrides have been resolved.
 */
type Endpoint struct {
	Host     string
	Port     int
	User     string
	Password string
	Database string
	SSLMode  string
}

/* EndpointFromConfig converts config.Connection to Endpoint. databaseOverride,
 * when non-empty, wins over the configured database. ProxyURL is intentionally
 * ignored because proxy routing is handled by Phase-1 dialer plumbing.
 */
func EndpointFromConfig(c config.Connection, databaseOverride string) Endpoint {
	database := c.Database
	if databaseOverride != "" {
		database = databaseOverride
	}
	return Endpoint{
		Host:     c.Host,
		Port:     c.Port,
		User:     c.User,
		Password: c.Password,
		Database: database,
		SSLMode:  c.SSLMode,
	}
}

/* BuildConnString builds a PostgreSQL URL connection string suitable for pgx. */
func BuildConnString(ep Endpoint) (string, error) {
	if strings.TrimSpace(ep.Host) == "" {
		return "", errors.New("postgres host is required")
	}
	if ep.Port < 1 || ep.Port > 65535 {
		return "", fmt.Errorf("postgres port out of range: %d", ep.Port)
	}

	database := ep.Database
	if database == "" {
		database = defaultMaintenanceDatabase
	}

	connURL := url.URL{
		Scheme: "postgres",
		Host:   net.JoinHostPort(ep.Host, strconv.Itoa(ep.Port)),
		Path:   "/" + database,
	}
	if ep.User != "" {
		if ep.Password != "" {
			connURL.User = url.UserPassword(ep.User, ep.Password)
		} else {
			connURL.User = url.User(ep.User)
		}
	}
	if ep.SSLMode != "" {
		query := connURL.Query()
		query.Set("sslmode", ep.SSLMode)
		connURL.RawQuery = query.Encode()
	}
	return connURL.String(), nil
}

/* MaskConnString redacts passwords from URL and keyword PostgreSQL connection
 * strings before they are included in logs or errors.
 */
func MaskConnString(connString string) string {
	if connString == "" {
		return ""
	}
	parsed, err := url.Parse(connString)
	if err == nil && parsed.User != nil {
		username := parsed.User.Username()
		if _, ok := parsed.User.Password(); ok {
			parsed.User = url.UserPassword(username, maskedPassword)
			connString = parsed.String()
		}
	}
	return passwordAssignmentPattern.ReplaceAllString(connString, `${1}`+maskedPassword)
}

package native

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"strconv"
	"strings"

	"github.com/mttzzz/pgsync/internal/engine/pgtools"
	"github.com/mttzzz/pgsync/internal/pgdb"
	"github.com/mttzzz/pgsync/internal/proxy"
	"github.com/mttzzz/pgsync/internal/runner"
)

const redactedSecret = "xxxxx"

// SchemaSection selects which pg_dump schema section to dump.
type SchemaSection string

const (
	// SchemaPreData contains objects that must exist before table data is copied.
	SchemaPreData SchemaSection = "pre-data"
	// SchemaPostData contains objects that should be applied after table data is copied.
	SchemaPostData SchemaSection = "post-data"
)

// SchemaDumper dumps PostgreSQL schema DDL through pg_dump.
type SchemaDumper struct {
	Runner  runner.CommandRunner
	Locator pgtools.Locator
}

// Dump returns plain SQL for the requested pg_dump schema section.
func (d *SchemaDumper) Dump(ctx context.Context, source pgdb.Endpoint, section SchemaSection) (string, error) {
	if err := validateSchemaSection(section); err != nil {
		return "", err
	}
	if d == nil {
		return "", errors.New("schema dumper is required")
	}
	if d.Runner == nil {
		return "", errors.New("schema dumper runner is required")
	}
	if d.Locator == nil {
		return "", errors.New("schema dumper locator is required")
	}

	dumpSource := source
	cleanup := func() {}
	if source.ProxyURL != "" {
		proxiedSource, closeTunnel, err := startPgDumpProxyTunnel(ctx, source)
		if err != nil {
			return "", err
		}
		dumpSource = proxiedSource
		cleanup = closeTunnel
	}
	defer cleanup()

	connString, err := pgdb.BuildConnString(dumpSource)
	if err != nil {
		return "", fmt.Errorf("build source connection string: %w", err)
	}
	pgDump, err := d.Locator.PgDump()
	if err != nil {
		return "", fmt.Errorf("locate pg_dump: %w", err)
	}

	args := []string{
		"--schema-only",
		"--no-owner",
		"--no-acl",
		"--format=plain",
		"--section=" + string(section),
		connString,
	}
	stdout, stderr, err := d.Runner.Run(ctx, pgDump, args, nil)
	if err != nil {
		return "", pgDumpError(section, source, connString, stderr, err)
	}
	sql := stripPgDumpMetaCommands(string(stdout))
	if section == SchemaPreData && strings.TrimSpace(sql) == "" {
		return "", errors.New("pg_dump returned empty pre-data schema")
	}
	return sql, nil
}

func startPgDumpProxyTunnel(ctx context.Context, source pgdb.Endpoint) (pgdb.Endpoint, func(), error) {
	dialer, err := proxy.NewDialer(source.ProxyURL)
	if err != nil {
		return pgdb.Endpoint{}, nil, fmt.Errorf("init pg_dump proxy: %w", err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return pgdb.Endpoint{}, nil, fmt.Errorf("listen pg_dump proxy tunnel: %w", err)
	}

	remoteAddr := net.JoinHostPort(source.Host, strconv.Itoa(source.Port))
	done := make(chan struct{})
	go acceptPgDumpProxyTunnel(ctx, listener, dialer, remoteAddr, done)

	proxied := source
	proxied.Host = "127.0.0.1"
	proxied.Port = listener.Addr().(*net.TCPAddr).Port
	proxied.ProxyURL = ""
	cleanup := func() {
		_ = listener.Close()
		<-done
	}
	return proxied, cleanup, nil
}

func acceptPgDumpProxyTunnel(ctx context.Context, listener net.Listener, dialer proxy.Dialer, remoteAddr string, done chan<- struct{}) {
	defer close(done)
	for {
		localConn, err := listener.Accept()
		if err != nil {
			return
		}
		go bridgePgDumpProxyConn(ctx, localConn, dialer, remoteAddr)
	}
}

func bridgePgDumpProxyConn(ctx context.Context, localConn net.Conn, dialer proxy.Dialer, remoteAddr string) {
	remoteConn, err := dialer.DialContext(ctx, "tcp", remoteAddr)
	if err != nil {
		_ = localConn.Close()
		return
	}
	defer func() { _ = localConn.Close() }()
	defer func() { _ = remoteConn.Close() }()

	copyDone := make(chan struct{}, 2)
	go func() {
		_, _ = io.Copy(remoteConn, localConn)
		copyDone <- struct{}{}
	}()
	go func() {
		_, _ = io.Copy(localConn, remoteConn)
		copyDone <- struct{}{}
	}()
	<-copyDone
}

func stripPgDumpMetaCommands(sql string) string {
	lines := strings.SplitAfter(sql, "\n")
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), `\`) {
			continue
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "")
}

func validateSchemaSection(section SchemaSection) error {
	switch section {
	case SchemaPreData, SchemaPostData:
		return nil
	default:
		return fmt.Errorf("unsupported schema section %q", section)
	}
}

func pgDumpError(section SchemaSection, source pgdb.Endpoint, connString string, stderr []byte, err error) error {
	message := fmt.Sprintf(
		"dump %s schema with pg_dump failed: %s",
		section,
		redactEndpointText(err.Error(), source, connString),
	)
	if strings.TrimSpace(string(stderr)) != "" {
		message += ": stderr: " + redactEndpointText(string(stderr), source, connString)
	}
	return errors.New(message)
}

func redactEndpointText(text string, endpoint pgdb.Endpoint, connString string) string {
	redacted := pgdb.MaskConnString(text)
	if connString != "" {
		redacted = strings.ReplaceAll(redacted, connString, pgdb.MaskConnString(connString))
	}
	if endpoint.Password != "" {
		redacted = strings.ReplaceAll(redacted, endpoint.Password, redactedSecret)
		redacted = strings.ReplaceAll(redacted, url.QueryEscape(endpoint.Password), redactedSecret)
		redacted = strings.ReplaceAll(redacted, url.PathEscape(endpoint.Password), redactedSecret)
	}
	return redacted
}

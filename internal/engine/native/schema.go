package native

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
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
	pgDump, connString, closeTunnel, err := d.prepare(ctx, source)
	if err != nil {
		return "", err
	}
	defer closeTunnel()

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
		return "", pgDumpError(string(section)+" schema", source, connString, stderr, err)
	}
	sql := stripPgDumpMetaCommands(string(stdout))
	if section == SchemaPreData && strings.TrimSpace(sql) == "" {
		return "", errors.New("pg_dump returned empty pre-data schema")
	}
	return sql, nil
}

// DumpArchive dumps the whole schema (pre-data and post-data) into a temporary custom-format
// archive for ArchiveRestorer. It returns the archive path and a func that deletes the archive;
// the caller owns the cleanup once the error is nil.
//
// The archive is deliberately NOT cut to --section=post-data. pg_restore --jobs keeps two
// workers off the same tables by the TABLE entries of the archive, and a post-data-only archive
// has none. Without them ALTER TABLE ... ADD FOREIGN KEY workers take their ShareRowExclusive
// locks (referencing table first, referenced second) in whatever order they start, so foreign
// keys that point at each other end in "deadlock detected" (measured on 8 such table pairs with
// --jobs=8: 18 of 40 restores failed, none of 60 with the full archive). Planning rejects FK
// cycles only among the tables being synced, while post-data covers the whole schema (a
// --tables run copies a subset).
// The section is picked at restore time instead (ArchiveRestorer.Restore).
func (d *SchemaDumper) DumpArchive(ctx context.Context, source pgdb.Endpoint) (string, func(), error) {
	pgDump, connString, closeTunnel, err := d.prepare(ctx, source)
	if err != nil {
		return "", nil, err
	}
	defer closeTunnel()

	file, err := os.CreateTemp("", "pgsync-schema-*.dump")
	if err != nil {
		return "", nil, fmt.Errorf("create schema archive file: %w", err)
	}
	path := file.Name()
	removeArchive := func() { _ = os.Remove(path) }
	// Only the unique name is needed: pg_dump creates the archive itself.
	_ = file.Close()

	args := []string{
		"--schema-only",
		"--no-owner",
		"--no-acl",
		"--format=custom",
		"--file=" + path,
		connString,
	}
	if _, stderr, err := d.Runner.Run(ctx, pgDump, args, nil); err != nil {
		removeArchive()
		return "", nil, pgDumpError("schema archive", source, connString, stderr, err)
	}
	return path, removeArchive, nil
}

// prepare validates the dumper and resolves what every pg_dump call needs besides its own flags:
// the pg_dump binary, the connection string and the closer of the proxy tunnel behind it.
func (d *SchemaDumper) prepare(ctx context.Context, source pgdb.Endpoint) (string, string, func(), error) {
	if d == nil {
		return "", "", nil, errors.New("schema dumper is required")
	}
	if d.Runner == nil {
		return "", "", nil, errors.New("schema dumper runner is required")
	}
	if d.Locator == nil {
		return "", "", nil, errors.New("schema dumper locator is required")
	}
	return preparePgTool(ctx, source, "source", "pg_dump", d.Locator.PgDump)
}

// ArchiveRestorer restores a section of a pg_dump custom-format archive through pg_restore.
type ArchiveRestorer struct {
	Runner  runner.CommandRunner
	Locator pgtools.Locator
}

// Restore restores one section of the archive at archivePath into target with jobs parallel
// pg_restore workers, on top of whatever the target database already has (the post-data section
// goes on top of the applied pre-data). The first error aborts the restore.
func (r *ArchiveRestorer) Restore(
	ctx context.Context,
	target pgdb.Endpoint,
	archivePath string,
	section SchemaSection,
	jobs int,
) error {
	if err := validateSchemaSection(section); err != nil {
		return err
	}
	if r == nil {
		return errors.New("archive restorer is required")
	}
	if r.Runner == nil {
		return errors.New("archive restorer runner is required")
	}
	if r.Locator == nil {
		return errors.New("archive restorer locator is required")
	}
	pgRestore, connString, closeTunnel, err := preparePgTool(ctx, target, "target", "pg_restore", r.Locator.PgRestore)
	if err != nil {
		return err
	}
	defer closeTunnel()

	args := []string{
		"--no-owner",
		"--no-acl",
		"--exit-on-error",
		"--section=" + string(section),
		"--jobs=" + strconv.Itoa(workerCount(jobs)),
		"--dbname=" + connString,
		archivePath,
	}
	if _, stderr, err := r.Runner.Run(ctx, pgRestore, args, nil); err != nil {
		return pgRestoreError(target, connString, stderr, err)
	}
	return nil
}

// preparePgTool resolves what a pg_dump or pg_restore call needs besides its own flags. Neither
// tool can dial a proxy, so an endpoint behind one gets a local TCP tunnel in front of it. The
// returned closer stops that tunnel; it is non-nil on success and must run once the tool has exited.
func preparePgTool(
	ctx context.Context,
	endpoint pgdb.Endpoint,
	role string,
	tool string,
	locate func() (string, error),
) (string, string, func(), error) {
	reached := endpoint
	closeTunnel := func() {}
	if endpoint.ProxyURL != "" {
		proxied, closeProxy, err := startPgToolProxyTunnel(ctx, endpoint)
		if err != nil {
			return "", "", nil, err
		}
		reached, closeTunnel = proxied, closeProxy
	}

	connString, err := pgdb.BuildConnString(reached)
	if err != nil {
		closeTunnel()
		return "", "", nil, fmt.Errorf("build %s connection string: %w", role, err)
	}
	path, err := locate()
	if err != nil {
		closeTunnel()
		return "", "", nil, fmt.Errorf("locate %s: %w", tool, err)
	}
	return path, connString, closeTunnel, nil
}

func startPgToolProxyTunnel(ctx context.Context, endpoint pgdb.Endpoint) (pgdb.Endpoint, func(), error) {
	dialer, err := proxy.NewDialer(endpoint.ProxyURL)
	if err != nil {
		return pgdb.Endpoint{}, nil, fmt.Errorf("init proxy tunnel: %w", err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return pgdb.Endpoint{}, nil, fmt.Errorf("listen proxy tunnel: %w", err)
	}

	remoteAddr := net.JoinHostPort(endpoint.Host, strconv.Itoa(endpoint.Port))
	done := make(chan struct{})
	go acceptPgToolProxyTunnel(ctx, listener, dialer, remoteAddr, done)

	proxied := endpoint
	proxied.Host = "127.0.0.1"
	proxied.Port = listener.Addr().(*net.TCPAddr).Port
	proxied.ProxyURL = ""
	cleanup := func() {
		_ = listener.Close()
		<-done
	}
	return proxied, cleanup, nil
}

func acceptPgToolProxyTunnel(ctx context.Context, listener net.Listener, dialer proxy.Dialer, remoteAddr string, done chan<- struct{}) {
	defer close(done)
	for {
		localConn, err := listener.Accept()
		if err != nil {
			return
		}
		go bridgePgToolProxyConn(ctx, localConn, dialer, remoteAddr)
	}
}

func bridgePgToolProxyConn(ctx context.Context, localConn net.Conn, dialer proxy.Dialer, remoteAddr string) {
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

func pgDumpError(what string, source pgdb.Endpoint, connString string, stderr []byte, err error) error {
	return pgToolError(fmt.Sprintf("dump %s with pg_dump failed", what), source, connString, stderr, err)
}

func pgRestoreError(target pgdb.Endpoint, connString string, stderr []byte, err error) error {
	return pgToolError("pg_restore failed", target, connString, stderr, err)
}

func pgToolError(summary string, endpoint pgdb.Endpoint, connString string, stderr []byte, err error) error {
	message := summary + ": " + redactEndpointText(err.Error(), endpoint, connString)
	if strings.TrimSpace(string(stderr)) != "" {
		message += ": stderr: " + redactEndpointText(string(stderr), endpoint, connString)
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

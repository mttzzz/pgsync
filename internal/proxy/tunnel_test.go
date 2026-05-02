package proxy_test

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pgproxy "github.com/mttzzz/pgsync/internal/proxy"
)

type fakeDialer struct {
	calls atomic.Int32
	conn  net.Conn
	err   error
}

func (f *fakeDialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	f.calls.Add(1)
	if f.err != nil {
		return nil, f.err
	}
	return f.conn, nil
}

func TestNewDirectDialerNoProxy(t *testing.T) {
	t.Parallel()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer func() { _ = ln.Close() }()
	done := make(chan struct{})
	go func() {
		conn, acceptErr := ln.Accept()
		if acceptErr == nil {
			_ = conn.Close()
		}
		close(done)
	}()

	dialer, err := pgproxy.NewDialer("")
	require.NoError(t, err)
	conn, err := dialer.DialContext(t.Context(), "tcp", ln.Addr().String())
	require.NoError(t, err)
	require.NoError(t, conn.Close())
	<-done
}

func TestNewDialerSocks5(t *testing.T) {
	t.Parallel()
	dialer, err := pgproxy.NewDialer("socks5://user:pass@localhost:1080")
	require.NoError(t, err)
	assert.NotNil(t, dialer)
}

func TestNewDialerHTTPConnectSuccess(t *testing.T) {
	t.Parallel()
	addr, stop := startHTTPProxy(t, "HTTP/1.1 200 Connection Established\r\n\r\n")
	defer stop()

	dialer, err := pgproxy.NewDialer("http://" + addr)
	require.NoError(t, err)
	conn, err := dialer.DialContext(t.Context(), "tcp", "db.example:5432")
	require.NoError(t, err)
	require.NoError(t, conn.Close())
}

func TestNewDialerHTTPConnectFailureStatus(t *testing.T) {
	t.Parallel()
	addr, stop := startHTTPProxy(t, "HTTP/1.1 403 Forbidden\r\n\r\n")
	defer stop()

	dialer, err := pgproxy.NewDialer("http://" + addr)
	require.NoError(t, err)
	_, err = dialer.DialContext(t.Context(), "tcp", "db.example:5432")
	require.Error(t, err)
}

func TestNewDialerInvalidScheme(t *testing.T) {
	t.Parallel()
	_, err := pgproxy.NewDialer("ftp://nope")
	require.Error(t, err)
}

func TestNewDialerInvalidURL(t *testing.T) {
	t.Parallel()
	_, err := pgproxy.NewDialer("://broken")
	require.Error(t, err)
}

func TestNewDialerMissingHost(t *testing.T) {
	t.Parallel()
	_, err := pgproxy.NewDialer("http:///missing-host")
	require.Error(t, err)
}

func TestTunnelDialFailure(t *testing.T) {
	t.Parallel()
	fake := &fakeDialer{err: errors.New("boom")}
	tunnel := pgproxy.NewTunnel(fake, "host:5432")

	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	_, err := tunnel.Dial(ctx)
	require.Error(t, err)
	assert.EqualValues(t, 1, fake.calls.Load())
}

func TestTunnelDialSuccess(t *testing.T) {
	t.Parallel()
	client, server := net.Pipe()
	defer func() { _ = server.Close() }()
	fake := &fakeDialer{conn: client}
	tunnel := pgproxy.NewTunnel(fake, "host:5432")

	conn, err := tunnel.Dial(t.Context())
	require.NoError(t, err)
	assert.Same(t, client, conn)
	require.NoError(t, conn.Close())
}

func startHTTPProxy(t *testing.T, response string) (string, func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, acceptErr := ln.Accept()
		if acceptErr != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		reader := bufio.NewReader(conn)
		line, readErr := reader.ReadString('\n')
		if readErr != nil || !strings.HasPrefix(line, "CONNECT db.example:5432") {
			return
		}
		_, _ = fmt.Fprint(conn, response)
	}()
	return ln.Addr().String(), func() {
		_ = ln.Close()
		<-done
	}
}

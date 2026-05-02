package proxy

import (
	"context"
	"errors"
	"net"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	xproxy "golang.org/x/net/proxy"
)

func TestNewSocksDialerWithFactoryBranches(t *testing.T) {
	t.Parallel()
	parsedWithAuth := mustURL(t, "socks5://user:pass@proxy:1080")
	var gotAuth *xproxy.Auth
	dialer, err := newSocksDialerWithFactory(parsedWithAuth, func(network, addr string, auth *xproxy.Auth, forward xproxy.Dialer) (xproxy.Dialer, error) {
		assert.Equal(t, "tcp", network)
		assert.Equal(t, "proxy:1080", addr)
		gotAuth = auth
		return fakeContextProxyDialer{}, nil
	})
	require.NoError(t, err)
	assert.Equal(t, "user", gotAuth.User)
	assert.Equal(t, "pass", gotAuth.Password)

	conn, err := dialer.DialContext(t.Context(), "tcp", "db:5432")
	require.NoError(t, err)
	require.NoError(t, conn.Close())
}

func TestNewSocksDialerWithFactoryNoPassword(t *testing.T) {
	t.Parallel()
	parsed := mustURL(t, "socks5://proxy:1080")
	_, err := newSocksDialerWithFactory(parsed, func(network, addr string, auth *xproxy.Auth, forward xproxy.Dialer) (xproxy.Dialer, error) {
		assert.Nil(t, auth)
		return fakeContextProxyDialer{}, nil
	})
	require.NoError(t, err)
}

func TestNewSocksDialerWithFactoryError(t *testing.T) {
	t.Parallel()
	parsed := mustURL(t, "socks5://proxy:1080")
	_, err := newSocksDialerWithFactory(parsed, func(network, addr string, auth *xproxy.Auth, forward xproxy.Dialer) (xproxy.Dialer, error) {
		return nil, errors.New("boom")
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "init socks5")
}

func TestNewSocksDialerWithFactoryNonContext(t *testing.T) {
	t.Parallel()
	parsed := mustURL(t, "socks5://proxy:1080")
	_, err := newSocksDialerWithFactory(parsed, func(network, addr string, auth *xproxy.Auth, forward xproxy.Dialer) (xproxy.Dialer, error) {
		return fakeProxyDialer{}, nil
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "context-aware")
}

func TestHTTPConnectDialerErrorBranches(t *testing.T) {
	t.Parallel()
	proxyURL := mustURL(t, "http://proxy:8080")
	boom := errors.New("boom")

	dialErr := &httpConnectDialer{
		proxyURL: proxyURL,
		dialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return nil, boom
		},
	}
	_, err := dialErr.DialContext(t.Context(), "tcp", "db:5432")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "dial http proxy")

	writeErr := &httpConnectDialer{
		proxyURL: proxyURL,
		dialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return &writeErrorConn{err: boom}, nil
		},
	}
	_, err = writeErr.DialContext(t.Context(), "tcp", "db:5432")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "write CONNECT")

	readErr := &httpConnectDialer{
		proxyURL: proxyURL,
		dialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return &readErrorConn{err: boom}, nil
		},
	}
	_, err = readErr.DialContext(t.Context(), "tcp", "db:5432")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "read CONNECT response")
}

func mustURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	parsed, err := url.Parse(raw)
	require.NoError(t, err)
	return parsed
}

type fakeProxyDialer struct{}

func (fakeProxyDialer) Dial(network, addr string) (net.Conn, error) {
	return nil, nil
}

type fakeContextProxyDialer struct{}

func (fakeContextProxyDialer) Dial(network, addr string) (net.Conn, error) {
	return nil, nil
}

func (fakeContextProxyDialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	client, server := net.Pipe()
	_ = server.Close()
	return client, nil
}

type writeErrorConn struct {
	err error
}

func (w *writeErrorConn) Read(b []byte) (int, error)         { return 0, errors.New("read unused") }
func (w *writeErrorConn) Write(b []byte) (int, error)        { return 0, w.err }
func (w *writeErrorConn) Close() error                       { return nil }
func (w *writeErrorConn) LocalAddr() net.Addr                { return fakeAddr("local") }
func (w *writeErrorConn) RemoteAddr() net.Addr               { return fakeAddr("remote") }
func (w *writeErrorConn) SetDeadline(t time.Time) error      { return nil }
func (w *writeErrorConn) SetReadDeadline(t time.Time) error  { return nil }
func (w *writeErrorConn) SetWriteDeadline(t time.Time) error { return nil }

type readErrorConn struct {
	err error
}

func (r *readErrorConn) Read(b []byte) (int, error)         { return 0, r.err }
func (r *readErrorConn) Write(b []byte) (int, error)        { return len(b), nil }
func (r *readErrorConn) Close() error                       { return nil }
func (r *readErrorConn) LocalAddr() net.Addr                { return fakeAddr("local") }
func (r *readErrorConn) RemoteAddr() net.Addr               { return fakeAddr("remote") }
func (r *readErrorConn) SetDeadline(t time.Time) error      { return nil }
func (r *readErrorConn) SetReadDeadline(t time.Time) error  { return nil }
func (r *readErrorConn) SetWriteDeadline(t time.Time) error { return nil }

type fakeAddr string

func (f fakeAddr) Network() string { return string(f) }
func (f fakeAddr) String() string  { return string(f) }

// Package proxy resolves proxy URLs into context-aware dialers.
package proxy

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"

	xproxy "golang.org/x/net/proxy"
)

/* Dialer opens context-aware network connections. */
type Dialer interface {
	DialContext(ctx context.Context, network, addr string) (net.Conn, error)
}

type directDialer struct{ d net.Dialer }

func (dd *directDialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	return dd.d.DialContext(ctx, network, addr)
}

/* NewDialer returns a Dialer that respects rawURL. Empty string means direct.
 * Supported schemes: socks5, socks5h, http, https. HTTP(S) uses CONNECT.
 */
func NewDialer(rawURL string) (Dialer, error) {
	if rawURL == "" {
		return &directDialer{}, nil
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("parse proxy url: %w", err)
	}
	if parsed.Host == "" {
		return nil, errors.New("proxy url has no host")
	}
	switch parsed.Scheme {
	case "socks5", "socks5h":
		return newSocksDialer(parsed)
	case "http", "https":
		return newHTTPConnectDialer(parsed), nil
	default:
		return nil, fmt.Errorf("unsupported proxy scheme %q", parsed.Scheme)
	}
}

type socksFactory func(network, addr string, auth *xproxy.Auth, forward xproxy.Dialer) (xproxy.Dialer, error)

func newSocksDialer(parsed *url.URL) (Dialer, error) {
	return newSocksDialerWithFactory(parsed, xproxy.SOCKS5)
}

func newSocksDialerWithFactory(parsed *url.URL, factory socksFactory) (Dialer, error) {
	var auth *xproxy.Auth
	if password, ok := parsed.User.Password(); ok {
		auth = &xproxy.Auth{User: parsed.User.Username(), Password: password}
	}
	dialer, err := factory("tcp", parsed.Host, auth, &net.Dialer{})
	if err != nil {
		return nil, fmt.Errorf("init socks5: %w", err)
	}
	contextDialer, ok := dialer.(xproxy.ContextDialer)
	if !ok {
		return nil, errors.New("socks5 dialer is not context-aware")
	}
	return socksAdapter{d: contextDialer}, nil
}

type socksAdapter struct{ d xproxy.ContextDialer }

func (s socksAdapter) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	return s.d.DialContext(ctx, network, addr)
}

type contextDialFunc func(ctx context.Context, network, addr string) (net.Conn, error)

type httpConnectDialer struct {
	proxyURL    *url.URL
	dialContext contextDialFunc
}

func newHTTPConnectDialer(proxyURL *url.URL) *httpConnectDialer {
	dialer := &net.Dialer{}
	return &httpConnectDialer{proxyURL: proxyURL, dialContext: dialer.DialContext}
}

func (h *httpConnectDialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	conn, err := h.dialContext(ctx, network, h.proxyURL.Host)
	if err != nil {
		return nil, fmt.Errorf("dial http proxy: %w", err)
	}
	request := (&http.Request{Method: http.MethodConnect, URL: &url.URL{Opaque: addr}, Host: addr, Header: make(http.Header)}).WithContext(ctx)
	if h.proxyURL.User != nil {
		password, _ := h.proxyURL.User.Password()
		request.SetBasicAuth(h.proxyURL.User.Username(), password)
		request.Header.Set("Proxy-Authorization", request.Header.Get("Authorization"))
		request.Header.Del("Authorization")
	}
	if err := request.Write(conn); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("write CONNECT: %w", err)
	}
	response, err := http.ReadResponse(bufio.NewReader(conn), request)
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("read CONNECT response: %w", err)
	}
	if response.StatusCode != http.StatusOK {
		_ = conn.Close()
		return nil, fmt.Errorf("CONNECT failed: %s", response.Status)
	}
	return conn, nil
}

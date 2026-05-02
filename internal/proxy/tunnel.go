package proxy

import (
	"context"
	"net"
)

/* Tunnel wraps a Dialer and remote address. */
type Tunnel struct {
	dialer Dialer
	addr   string
}

/* NewTunnel creates a tunnel dial helper. */
func NewTunnel(dialer Dialer, remoteAddr string) *Tunnel {
	return &Tunnel{dialer: dialer, addr: remoteAddr}
}

/* Dial opens a TCP connection to the configured remote address. */
func (t *Tunnel) Dial(ctx context.Context) (net.Conn, error) {
	return t.dialer.DialContext(ctx, "tcp", t.addr)
}

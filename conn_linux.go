//+build linux

package vsock

import (
	"net"
	"time"

	"golang.org/x/sys/unix"
)

var _ net.Conn = &conn{}

// A conn is the net.Conn implementation for VM sockets.
type conn struct {
	fd         connFD
	localAddr  *Addr
	remoteAddr *Addr
}

// Implement net.Conn for type conn.
func (c *conn) LocalAddr() net.Addr                { return c.localAddr }
func (c *conn) RemoteAddr() net.Addr               { return c.remoteAddr }
func (c *conn) SetDeadline(t time.Time) error      { return c.fd.SetDeadline(t) }
func (c *conn) SetReadDeadline(t time.Time) error  { return c.fd.SetReadDeadline(t) }
func (c *conn) SetWriteDeadline(t time.Time) error { return c.fd.SetWriteDeadline(t) }
func (c *conn) Read(b []byte) (n int, err error)   { return c.fd.Read(b) }
func (c *conn) Write(b []byte) (n int, err error)  { return c.fd.Write(b) }
func (c *conn) Close() error                       { return c.fd.Close() }

// newConn creates a conn using an connFD, immediately setting the connFD to
// non-blocking mode for use with the runtime network poller.
func newConn(cfd connFD, local, remote *Addr) (*conn, error) {
	// Note: if any calls fail after this point, cfd.Close should be invoked
	// for cleanup because the socket is now non-blocking.
	if err := cfd.SetNonblocking(local.fileName()); err != nil {
		return nil, err
	}

	return &conn{
		fd:         cfd,
		localAddr:  local,
		remoteAddr: remote,
	}, nil
}

// dialStream is the entry point for DialStream on Linux.
func dialStream(cid, port uint32) (net.Conn, error) {
	cfd, err := newConnFD()
	if err != nil {
		return nil, err
	}

	return dialStreamLinuxHandleError(cfd, cid, port)
}

// dialStreamLinuxHandleError ensures that any errors from dialStreamLinux result
// in the socket being cleaned up properly.
func dialStreamLinuxHandleError(cfd connFD, cid, port uint32) (net.Conn, error) {
	c, err := dialStreamLinux(cfd, cid, port)
	if err != nil {
		// If any system calls fail during setup, the socket must be closed
		// early to avoid file descriptor leaks.
		_ = cfd.EarlyClose()
		return nil, err
	}

	return c, nil
}

// dialStreamLinux is the entry point for tests on Linux.
func dialStreamLinux(cfd connFD, cid, port uint32) (net.Conn, error) {
	rsa := &unix.SockaddrVM{
		CID:  cid,
		Port: port,
	}

	if err := cfd.Connect(rsa); err != nil {
		return nil, err
	}

	lsa, err := cfd.Getsockname()
	if err != nil {
		return nil, err
	}

	lsavm := lsa.(*unix.SockaddrVM)

	local := &Addr{
		ContextID: lsavm.CID,
		Port:      lsavm.Port,
	}

	remote := &Addr{
		ContextID: cid,
		Port:      port,
	}

	return newConn(cfd, local, remote)
}

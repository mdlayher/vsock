// Package vsock provides access to Linux VM sockets (AF_VSOCK) for
// communication between a hypervisor and its virtual machines.
package vsock

import (
	"fmt"
	"net"
	"time"
)

const (
	// Hypervisor specifies that a socket should communicate with the hypervisor
	// process.
	Hypervisor = 0x0

	// Host specifies that a socket should communicate with processes other than
	// the hypervisor on the host machine.
	Host = 0x2

	// cidReserved is a reserved context ID that is no longer in use,
	// and cannot be used for socket communications.
	cidReserved = 0x1
)

// Listen opens a connection-oriented net.Listener for incoming VM sockets
// connections. The port parameter specifies the port for the Listener.
//
// To allow the server to assign a port automatically, specify 0 for port.
// The address of the server can be retrieved using the Addr method.
//
// When the Listener is no longer needed, Close must be called to free resources.
func Listen(port uint32) (*Listener, error) {
	return listenStream(port)
}

var _ net.Listener = &Listener{}

// A Listener is a VM sockets implementation of a net.Listener.
type Listener struct {
	l *listener
}

// Accept implements the Accept method in the net.Listener interface; it waits
// for the next call and returns a generic net.Conn. The returned net.Conn will
// always be of type *Conn.
func (l *Listener) Accept() (net.Conn, error) { return l.l.Accept() }

// Addr returns the listener's network address, a *Addr. The Addr returned is
// shared by all invocations of Addr, so do not modify it.
func (l *Listener) Addr() net.Addr { return l.l.Addr() }

// Close stops listening on the VM sockets address. Already Accepted connections
// are not closed.
func (l *Listener) Close() error { return l.l.Close() }

// SetDeadline sets the deadline associated with the listener. A zero time value
// disables the deadline.
//
// SetDeadline only works with Go 1.12+.
func (l *Listener) SetDeadline(t time.Time) error { return l.l.SetDeadline(t) }

// Dial dials a connection-oriented net.Conn to a VM sockets server.
// The contextID and port parameters specify the address of the server.
//
// If dialing a connection from the hypervisor to a virtual machine, the VM's
// context ID should be specified.
//
// If dialing from a VM to the hypervisor, Hypervisor should be used to
// communicate with the hypervisor process, or Host should be used to
// communicate with other processes on the host machine.
//
// When the connection is no longer needed, Close must be called to free resources.
func Dial(contextID, port uint32) (*Conn, error) {
	return dialStream(contextID, port)
}

var _ net.Conn = &Conn{}

// A Conn is a VM sockets implementation of a net.Conn.
type Conn struct {
	c *conn
}

// Close closes the connection.
func (c *Conn) Close() error { return c.c.Close() }

// LocalAddr returns the local network address. The Addr returned is shared by
// all invocations of LocalAddr, so do not modify it.
func (c *Conn) LocalAddr() net.Addr { return c.c.LocalAddr() }

// RemoteAddr returns the remote network address. The Addr returned is shared by
// all invocations of RemoteAddr, so do not modify it.
func (c *Conn) RemoteAddr() net.Addr { return c.c.RemoteAddr() }

// Read implements the net.Conn Read method.
func (c *Conn) Read(b []byte) (n int, err error) { return c.c.Read(b) }

// Write implements the net.Conn Write method.
func (c *Conn) Write(b []byte) (n int, err error) { return c.c.Write(b) }

// SetDeadline implements the net.Conn SetDeadline method.
func (c *Conn) SetDeadline(t time.Time) error { return c.c.SetDeadline(t) }

// SetReadDeadline implements the net.Conn SetReadDeadline method.
func (c *Conn) SetReadDeadline(t time.Time) error { return c.c.SetReadDeadline(t) }

// SetWriteDeadline implements the net.Conn SetWriteDeadline method.
func (c *Conn) SetWriteDeadline(t time.Time) error { return c.c.SetWriteDeadline(t) }

// TODO(mdlayher): ListenPacket and DialPacket (or maybe another parameter for Dial?).

var _ net.Addr = &Addr{}

// An Addr is the address of a VM sockets endpoint.
type Addr struct {
	ContextID uint32
	Port      uint32
}

// Network returns the address's network name, "vsock".
func (a *Addr) Network() string { return "vsock" }

// String returns a human-readable representation of Addr, and indicates if
// ContextID is meant to be used for a hypervisor, host, VM, etc.
func (a *Addr) String() string {
	var host string

	switch a.ContextID {
	case Hypervisor:
		host = fmt.Sprintf("hypervisor(%d)", a.ContextID)
	case cidReserved:
		host = fmt.Sprintf("reserved(%d)", a.ContextID)
	case Host:
		host = fmt.Sprintf("host(%d)", a.ContextID)
	default:
		host = fmt.Sprintf("vm(%d)", a.ContextID)
	}

	return fmt.Sprintf("%s:%d", host, a.Port)
}

// fileName returns a file name for use with os.NewFile for Addr.
func (a *Addr) fileName() string {
	return fmt.Sprintf("%s:%s", a.Network(), a.String())
}

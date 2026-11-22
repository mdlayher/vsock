package vsock

import (
	"net"
	"os"
	"sync"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
)

var _ net.Listener = &listener{}

// A listener is the net.Listener implementation for connection-oriented
// VM sockets.
type listener struct {
	fd   *netFD
	addr *Addr

	// Serialize accepts.
	mu sync.Mutex
}

// listen is the entry point for Listen on Windows.
func listen(contextID, port uint32, _ *Config) (*Listener, error) {
	fd, err := socket(afVsock, windows.SOCK_STREAM, 0)
	if err != nil {
		return nil, os.NewSyscallError("socket", err)
	}

	if port == 0 {
		port = vmaddrPortAny
	}

	sa := &sockaddrVM{
		family: afVsock,
		cid:    contextID,
		port:   port,
	}

	err = bind(fd, sa)
	if err != nil {
		_ = windows.Closesocket(fd)
		return nil, os.NewSyscallError("bind", err)
	}

	err = windows.Listen(fd, windows.SOMAXCONN)
	if err != nil {
		_ = windows.Closesocket(fd)
		return nil, os.NewSyscallError("listen", err)
	}

	// Retrieve the address as the port may have been assigned.
	addr, err := getsockname(fd)
	if err != nil {
		_ = windows.Closesocket(fd)
		return nil, os.NewSyscallError("getsockname", err)
	}

	nfd, err := newFD(fd)
	if err != nil {
		return nil, err
	}

	return &Listener{
		l: &listener{
			fd:   nfd,
			addr: addr,
		},
	}, nil
}

// fileListener is the entry point for FileListener on Windows.
func fileListener(_ *os.File) (*Listener, error) { return nil, errUnimplemented }

// Accept accepts a single connection from the listener, and sets up
// a net.Conn backed by conn.
func (l *listener) Accept() (net.Conn, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	err := l.fd.lock()
	if err != nil {
		return nil, err
	}
	defer l.fd.unlock()

	for {
		err := l.fd.check(modeRead)
		if err != nil {
			return nil, err
		}

		fd, peer, err := accept(l.fd.fd)
		if err == nil {
			// Accepted sockets share the events of the listener until
			// they get their own.
			nfd, err := newFD(fd)
			if err != nil {
				return nil, err
			}

			return &Conn{
				c:      &conn{fd: nfd},
				local:  l.addr,
				remote: &Addr{ContextID: peer.cid, Port: peer.port},
			}, nil
		}

		if err != syscall.EAGAIN {
			return nil, os.NewSyscallError("accept", err)
		}

		err = l.fd.wait(modeRead)
		if err != nil {
			return nil, err
		}
	}
}

func (l *listener) Addr() net.Addr { return l.addr }

func (l *listener) Close() error {
	return l.fd.close()
}

func (l *listener) SetDeadline(t time.Time) error {
	return l.fd.setDeadline(modeRead, t)
}

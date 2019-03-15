package vsock

import (
	"io"
	"os"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

// A listenFD is a type that wraps a file descriptor used to implement
// net.Listener.
type listenFD interface {
	io.Closer
	Accept4(flags int) (connFD, unix.Sockaddr, error)
	Bind(sa unix.Sockaddr) error
	Listen(n int) error
	Getsockname() (unix.Sockaddr, error)
}

var _ listenFD = &sysListenFD{}

// A sysListenFD is the system call implementation of listenFD.
type sysListenFD struct {
	// These fields should never be non-zero at the same time.
	fd int      // Used in blocking mode.
	f  *os.File // Used in non-blocking mode.
}

// newListenFD creates a sysListenFD in its default blocking mode.
func newListenFD() (*sysListenFD, error) {
	fd, err := unix.Socket(unix.AF_VSOCK, unix.SOCK_STREAM, 0)
	if err != nil {
		return nil, err
	}

	return &sysListenFD{
		fd: fd,
	}, nil
}

// Blocking mode methods.

func (lfd *sysListenFD) Bind(sa unix.Sockaddr) error {
	lfd.check()
	return unix.Bind(lfd.fd, sa)
}

func (lfd *sysListenFD) Getsockname() (unix.Sockaddr, error) {
	lfd.check()
	return unix.Getsockname(lfd.fd)
}

func (lfd *sysListenFD) Listen(n int) error {
	lfd.check()
	return unix.Listen(lfd.fd, n)
}

// Non-blocking mode methods.

func (lfd *sysListenFD) Accept4(flags int) (connFD, unix.Sockaddr, error) {
	// From now on, we must perform non-blocking I/O, so that our
	// net.Listener.Accept method can be interrupted by closing the socket.
	if err := unix.SetNonblock(lfd.fd, true); err != nil {
		return nil, nil, err
	}

	// Transition from blocking mode to non-blocking mode, and ensure invariants
	// are not violated.
	lfd.f = os.NewFile(uintptr(lfd.fd), "vsock-listen")
	lfd.fd = 0
	lfd.check()

	rc, err := lfd.f.SyscallConn()
	if err != nil {
		return nil, nil, err
	}

	var (
		nfd int
		sa  unix.Sockaddr
	)

	doErr := rc.Read(func(fd uintptr) bool {
		nfd, sa, err = unix.Accept4(int(fd), flags)

		// When the socket is in non-blocking mode, we might see
		// EAGAIN and end up here. In that case, return false to
		// let the poller wait for readiness. See the source code
		// for internal/poll.FD.RawRead for more details.
		return err != syscall.EAGAIN
	})
	if doErr != nil {
		return nil, nil, doErr
	}
	if err != nil {
		return nil, nil, err
	}

	// Create a non-blocking connFD which will be used to implement net.Conn.
	cfd, err := newConnFD(nfd)
	if err != nil {
		return nil, nil, err
	}

	return cfd, sa, nil
}

func (lfd *sysListenFD) Close() error {
	lfd.check()

	// It is possible that Close will be called before a transition to
	// non-blocking mode in Accept.
	if lfd.f == nil {
		return unix.Close(lfd.fd)
	}

	var err error
	doErr := fdcontrol(lfd.f, func(fd int) {
		err = unix.Close(fd)
	})
	if doErr != nil {
		return doErr
	}

	// We must also close the runtime network poller file descriptor for
	// net.Listener.Accept to stop blocking.
	_ = lfd.f.Close()
	return err
}

func (lfd *sysListenFD) check() {
	// Verify that both file descriptor states cannot exist at the same time.
	if lfd.fd != 0 && lfd.f != nil {
		panic("vsock: sysListenFD blocking to non-blocking mode transition invariant violation, please file a bug: https://github.com/mdlayher/vsock")
	}
}

// A fd is an interface for a file descriptor, used to perform system
// calls or swap them out for tests.
type connFD interface {
	io.ReadWriteCloser
	Getsockname() (unix.Sockaddr, error)
	SetDeadline(t time.Time) error
	SetReadDeadline(t time.Time) error
	SetWriteDeadline(t time.Time) error
}

var _ connFD = &sysConnFD{}

func newConnFD(fd int) (*sysConnFD, error) {
	if err := unix.SetNonblock(fd, true); err != nil {
		_ = unix.Close(fd)
		return nil, err
	}

	return &sysConnFD{
		f: os.NewFile(uintptr(fd), "vsock"),
	}, nil
}

// sysConnFD is the system call implementation of fd.
type sysConnFD struct {
	f *os.File
}

func (cfd *sysConnFD) Getsockname() (unix.Sockaddr, error) {
	var (
		sa  unix.Sockaddr
		err error
	)

	doErr := fdcontrol(cfd.f, func(fd int) {
		sa, err = unix.Getsockname(fd)
	})
	if doErr != nil {
		return nil, doErr
	}

	return sa, err
}

func (cfd *sysConnFD) File() *os.File {
	return cfd.f
}

func (cfd *sysConnFD) Close() error {
	var err error
	doErr := fdcontrol(cfd.f, func(fd int) {
		err = unix.Close(fd)
	})
	if doErr != nil {
		return doErr
	}

	_ = cfd.f.Close()

	return err
}

func (cfd *sysConnFD) Read(b []byte) (int, error) {
	n, err := cfd.f.Read(b)
	if err != nil {
		if perr, ok := err.(*os.PathError); ok && perr.Err == unix.ENOTCONN {
			return n, io.EOF
		}
	}

	return n, err
}

func (cfd *sysConnFD) Write(b []byte) (int, error)        { return cfd.f.Write(b) }
func (cfd *sysConnFD) SetDeadline(t time.Time) error      { return cfd.f.SetDeadline(t) }
func (cfd *sysConnFD) SetReadDeadline(t time.Time) error  { return cfd.f.SetReadDeadline(t) }
func (cfd *sysConnFD) SetWriteDeadline(t time.Time) error { return cfd.f.SetWriteDeadline(t) }

func fdcontrol(fd *os.File, f func(int)) error {
	rc, err := fd.SyscallConn()
	if err != nil {
		return err
	}
	return rc.Control(func(sysConnFD uintptr) {
		f(int(sysConnFD))
	})
}

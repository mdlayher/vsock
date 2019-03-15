package vsock

import (
	"io"
	"os"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

type listenFD interface {
	io.Closer
	Accept4(flags int) (connFD, unix.Sockaddr, error)
	Bind(sa unix.Sockaddr) error
	Listen(n int) error
	Getsockname() (unix.Sockaddr, error)
}

type sysListenFD struct {
	f *os.File
}

func newListenFD() (*sysListenFD, error) {
	fd, err := unix.Socket(unix.AF_VSOCK, unix.SOCK_STREAM, 0)
	if err != nil {
		return nil, err
	}

	if err := unix.SetNonblock(fd, true); err != nil {
		return nil, err
	}

	return &sysListenFD{
		f: os.NewFile(uintptr(fd), "vsock-listen"),
	}, nil
}

func (lfd *sysListenFD) Accept4(flags int) (connFD, unix.Sockaddr, error) {
	var (
		nfd int
		sa  unix.Sockaddr
		err error
	)

	doErr := fdread(lfd.f, func(fd int) bool {
		// Returns a regular file descriptor, must be wrapped in another
		// sysConnFD for it to work properly.
		nfd, sa, err = unix.Accept4(fd, flags)

		// When the socket is in non-blocking mode, we might see
		// EAGAIN and end up here. In that case, return false to
		// let the poller wait for readiness. See the source code
		// for internal/poll.FD.RawRead for more details.
		//
		// If the socket is in blocking mode, EAGAIN should never occur.
		return err != syscall.EAGAIN
	})
	if doErr != nil {
		return nil, nil, doErr
	}
	if err != nil {
		return nil, nil, err
	}

	cfd, err := newConnFD(nfd)
	if err != nil {
		return nil, nil, err
	}

	return cfd, sa, nil
}

func (lfd *sysListenFD) Bind(sa unix.Sockaddr) error {
	var err error
	doErr := fdcontrol(lfd.f, func(fd int) {
		err = unix.Bind(fd, sa)
	})
	if doErr != nil {
		return doErr
	}

	return err
}

func (lfd *sysListenFD) Close() error {
	var err error
	doErr := fdcontrol(lfd.f, func(fd int) {
		err = unix.Close(fd)
	})
	if doErr != nil {
		return doErr
	}

	_ = lfd.f.Close()
	return err
}

func (lfd *sysListenFD) Getsockname() (unix.Sockaddr, error) {
	var (
		sa  unix.Sockaddr
		err error
	)

	doErr := fdcontrol(lfd.f, func(fd int) {
		sa, err = unix.Getsockname(fd)
	})
	if doErr != nil {
		return nil, doErr
	}

	return sa, err
}

func (lfd *sysListenFD) Listen(n int) error {
	var err error
	doErr := fdcontrol(lfd.f, func(fd int) {
		err = unix.Listen(fd, n)
	})
	if doErr != nil {
		return doErr
	}

	return err
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

func fdread(fd *os.File, f func(int) bool) error {
	rc, err := fd.SyscallConn()
	if err != nil {
		return err
	}
	return rc.Read(func(sysConnFD uintptr) bool {
		return f(int(sysConnFD))
	})
}

func fdcontrol(fd *os.File, f func(int)) error {
	rc, err := fd.SyscallConn()
	if err != nil {
		return err
	}
	return rc.Control(func(sysConnFD uintptr) {
		f(int(sysConnFD))
	})
}

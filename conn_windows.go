package vsock

import (
	"context"
	"io"
	"os"
	"sync"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
)

// dial is the entry point for Dial on Windows.
func dial(ctx context.Context, contextID, port uint32, _ *Config) (*Conn, error) {
	err := ctx.Err()
	if err != nil {
		return nil, err
	}

	fd, err := socket(afVsock, windows.SOCK_STREAM, 0)
	if err != nil {
		return nil, os.NewSyscallError("socket", err)
	}

	addr := &sockaddrVM{
		family: afVsock,
		cid:    contextID,
		port:   port,
	}

	// A blocking connect can only be interrupted by closing the socket.
	done := make(chan struct{})
	go func() {
		err = connect(fd, addr)
		close(done)
	}()

	select {
	case <-done:
	case <-ctx.Done():
		_ = windows.Closesocket(fd)
		<-done

		return nil, ctx.Err()
	}

	if err != nil {
		_ = windows.Closesocket(fd)
		return nil, os.NewSyscallError("connect", err)
	}

	localAddr, err := getsockname(fd)
	if err != nil {
		_ = windows.Closesocket(fd)
		return nil, os.NewSyscallError("getsockname", err)
	}

	nfd, err := newFD(fd)
	if err != nil {
		return nil, err
	}

	return &Conn{
		c:      &conn{fd: nfd},
		local:  localAddr,
		remote: &Addr{ContextID: contextID, Port: port},
	}, nil
}

// A conn is the net.Conn implementation for connection-oriented VM sockets.
type conn struct {
	fd *netFD

	// Serialize reads and writes.
	r sync.Mutex
	w sync.Mutex
}

func (c *conn) Close() error {
	return c.fd.close()
}

func (c *conn) CloseRead() error {
	return c.shutdown(windows.SHUT_RD)
}

func (c *conn) CloseWrite() error {
	return c.shutdown(windows.SHUT_WR)
}

func (c *conn) shutdown(how int) error {
	err := c.fd.lock()
	if err != nil {
		return err
	}
	defer c.fd.unlock()

	return windows.Shutdown(c.fd.fd, how)
}

func (c *conn) Read(b []byte) (int, error) {
	c.r.Lock()
	defer c.r.Unlock()

	err := c.fd.lock()
	if err != nil {
		return 0, err
	}
	defer c.fd.unlock()

	for {
		err := c.fd.check(modeRead)
		if err != nil {
			return 0, err
		}

		if len(b) == 0 {
			return 0, nil
		}

		n, err := recv(c.fd.fd, b)
		if err == nil {
			if n == 0 {
				return 0, io.EOF
			}

			return n, nil
		}

		// Reading after CloseRead behaves as on other platforms.
		if err == windows.WSAESHUTDOWN {
			return 0, io.EOF
		}

		if err != syscall.EAGAIN {
			return 0, err
		}

		err = c.fd.wait(modeRead)
		if err != nil {
			return 0, err
		}
	}
}

func (c *conn) Write(b []byte) (int, error) {
	c.w.Lock()
	defer c.w.Unlock()

	err := c.fd.lock()
	if err != nil {
		return 0, err
	}
	defer c.fd.unlock()

	offset := 0
	for {
		err := c.fd.check(modeWrite)
		if err != nil {
			return offset, err
		}

		n, err := send(c.fd.fd, b[offset:])
		offset += n
		if err == nil {
			if offset == len(b) {
				return offset, nil
			}

			continue
		}

		if err != syscall.EAGAIN {
			return offset, err
		}

		err = c.fd.wait(modeWrite)
		if err != nil {
			return offset, err
		}
	}
}

func (c *conn) SetDeadline(t time.Time) error {
	err := c.fd.setDeadline(modeRead, t)
	if err != nil {
		return err
	}

	return c.fd.setDeadline(modeWrite, t)
}

func (c *conn) SetReadDeadline(t time.Time) error {
	return c.fd.setDeadline(modeRead, t)
}

func (c *conn) SetWriteDeadline(t time.Time) error {
	return c.fd.setDeadline(modeWrite, t)
}

func (c *conn) SyscallConn() (syscall.RawConn, error) {
	return nil, errUnimplemented
}

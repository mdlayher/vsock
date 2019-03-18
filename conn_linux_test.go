//+build linux

package vsock

import (
	"errors"
	"testing"

	"github.com/google/go-cmp/cmp"
	"golang.org/x/sys/unix"
)

func Test_dialStreamLinuxHandleError(t *testing.T) {
	var closed bool
	cfd := &testConnFD{
		// Track when fd.Close is called.
		close: func() error {
			closed = true
			return nil
		},
		// Always return an error on connect.
		connect: func(sa unix.Sockaddr) error {
			return errors.New("error during connect")
		},
	}

	if _, err := dialStreamLinuxHandleError(cfd, 0, 0); err == nil {
		t.Fatal("expected an error, but none occurred")
	}

	if diff := cmp.Diff(true, closed); diff != "" {
		t.Fatalf("unexpected closed value (-want +got):\n%s", diff)
	}
}

func Test_dialStreamLinuxFull(t *testing.T) {
	const (
		localCID  uint32 = 3
		localPort uint32 = 1024

		remoteCID  uint32 = Host
		remotePort uint32 = 2048
	)

	lsa := &unix.SockaddrVM{
		CID:  localCID,
		Port: localPort,
	}

	rsa := &unix.SockaddrVM{
		CID:  remoteCID,
		Port: remotePort,
	}

	var (
		closed      bool
		closedRead  bool
		closedWrite bool
	)

	cfd := &testConnFD{
		connect: func(sa unix.Sockaddr) error {
			if diff := cmp.Diff(rsa, sa.(*unix.SockaddrVM), cmp.AllowUnexported(*rsa)); diff != "" {
				t.Fatalf("unexpected connect sockaddr (-want +got):\n%s", diff)
			}

			return nil
		},
		getsockname: func() (unix.Sockaddr, error) {
			return lsa, nil
		},
		setNonblocking: func(name string) error {
			if diff := cmp.Diff(name, "vsock:vm(3):1024"); diff != "" {
				t.Fatalf("unexpected non-blocking file name (-want +got):\n%s", diff)
			}

			return nil
		},
		close: func() error {
			closed = true
			return nil
		},
		shutdown: func(how int) error {
			switch how {
			case unix.SHUT_RD:
				closedRead = true
			case unix.SHUT_WR:
				closedWrite = true
			default:
				t.Fatalf("unexpected how constant in shutdown: %d", how)
			}

			return nil
		},
	}

	c, err := dialStreamLinux(cfd, remoteCID, remotePort)
	if err != nil {
		t.Fatalf("failed to dial: %v", err)
	}

	localAddr := &Addr{
		ContextID: localCID,
		Port:      localPort,
	}

	if diff := cmp.Diff(localAddr, c.LocalAddr()); diff != "" {
		t.Fatalf("unexpected local address (-want +got):\n%s", diff)
	}

	remoteAddr := &Addr{
		ContextID: remoteCID,
		Port:      remotePort,
	}

	if diff := cmp.Diff(remoteAddr, c.RemoteAddr()); diff != "" {
		t.Fatalf("unexpected remote address (-want +got):\n%s", diff)
	}

	// Verify Close/Shutdown plumbing.
	funcs := []func() error{
		c.Close,
		c.CloseRead,
		c.CloseWrite,
	}

	for i, fn := range funcs {
		if err := fn(); err != nil {
			t.Fatalf("failed to invoke function %d: %v", i, err)
		}
	}

	if !closed || !closedRead || !closedWrite {
		t.Fatalf("expected calls to Close (%t), CloseRead (%t), and CloseWrite (%t)",
			closed, closedRead, closedWrite)
	}
}

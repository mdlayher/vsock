package vsock

import (
	"errors"
	"testing"

	"github.com/google/go-cmp/cmp"
	"golang.org/x/sys/unix"
)

func Test_listenStreamLinuxHandleError(t *testing.T) {
	var closed bool

	lfd := &testListenFD{
		// Track when fd.Close is called.
		close: func() error {
			closed = true
			return nil
		},
		// Always return an error on bind.
		bind: func(sa unix.Sockaddr) error {
			return errors.New("error during bind")
		},
	}

	if _, err := listenStreamLinuxHandleError(lfd, 0, 0); err == nil {
		t.Fatal("expected an error, but none occurred")
	}

	if diff := cmp.Diff(true, closed); diff != "" {
		t.Fatalf("unexpected closed value (-want +got):\n%s", diff)
	}
}

func Test_listenStreamLinuxPortZero(t *testing.T) {
	const (
		cid  uint32 = Host
		port uint32 = 0
	)

	lsa := &unix.SockaddrVM{
		CID: cid,
		// Expect 0 to be turned into "any port".
		Port: unix.VMADDR_PORT_ANY,
	}

	lfd := &testListenFD{
		bind: func(sa unix.Sockaddr) error {
			if diff := cmp.Diff(lsa, sa.(*unix.SockaddrVM), cmp.AllowUnexported(*lsa)); diff != "" {
				t.Fatalf("unexpected bind sockaddr (-want +got):\n%s", diff)
			}

			return nil
		},
		listen:         func(n int) error { return nil },
		getsockname:    func() (unix.Sockaddr, error) { return lsa, nil },
		setNonblocking: func(_ string) error { return nil },
	}

	if _, err := listenStreamLinux(lfd, cid, port); err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
}

func Test_listenStreamLinuxFull(t *testing.T) {
	const (
		cid  uint32 = Host
		port uint32 = 1024
	)

	lsa := &unix.SockaddrVM{
		CID:  cid,
		Port: port,
	}

	var nonblocking bool
	lfd := &testListenFD{
		bind: func(sa unix.Sockaddr) error {
			if diff := cmp.Diff(lsa, sa.(*unix.SockaddrVM), cmp.AllowUnexported(*lsa)); diff != "" {
				t.Fatalf("unexpected bind sockaddr (-want +got):\n%s", diff)
			}

			return nil
		},
		listen: func(n int) error {
			if diff := cmp.Diff(listenBacklog, n); diff != "" {
				t.Fatalf("unexpected listen backlog (-want +got):\n%s", diff)
			}

			return nil
		},
		getsockname: func() (unix.Sockaddr, error) {
			return lsa, nil
		},
		setNonblocking: func(_ string) error {
			nonblocking = true
			return nil
		},
	}

	l, err := listenStreamLinux(lfd, cid, port)
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}

	want := &Addr{
		ContextID: cid,
		Port:      port,
	}

	if diff := cmp.Diff(true, nonblocking); diff != "" {
		t.Fatalf("unexpected non-blocking value (-want +got):\n%s", diff)
	}

	if diff := cmp.Diff(want, l.Addr()); diff != "" {
		t.Fatalf("unexpected local address (-want +got):\n%s", diff)
	}
}

func Test_listenerAccept(t *testing.T) {
	const (
		cid  uint32 = 3
		port uint32 = 1024
	)

	var nonblocking bool
	accept4Fn := func(flags int) (connFD, unix.Sockaddr, error) {
		if diff := cmp.Diff(0, flags); diff != "" {
			t.Fatalf("unexpected accept4 flags (-want +got):\n%s", diff)
		}

		acceptFD := &testConnFD{
			setNonblocking: func(_ string) error {
				nonblocking = true
				return nil
			},
		}

		acceptSA := &unix.SockaddrVM{
			CID:  cid,
			Port: port,
		}

		return acceptFD, acceptSA, nil
	}

	localAddr := &Addr{
		ContextID: Host,
		Port:      port,
	}

	l := &listener{
		fd: &testListenFD{
			accept4: accept4Fn,
		},
		addr: localAddr,
	}

	nc, err := l.Accept()
	if err != nil {
		t.Fatalf("failed to accept: %v", err)
	}

	c := nc.(*conn)

	if !nonblocking {
		t.Fatal("file descriptor was not set to non-blocking mode")
	}

	if diff := cmp.Diff(localAddr, c.LocalAddr()); diff != "" {
		t.Fatalf("unexpected local address (-want +got):\n%s", diff)
	}

	remoteAddr := &Addr{
		ContextID: cid,
		Port:      port,
	}

	if diff := cmp.Diff(remoteAddr, c.RemoteAddr()); diff != "" {
		t.Fatalf("unexpected remote address (-want +got):\n%s", diff)
	}
}

//+build go1.12,linux

package vsock_test

import (
	"net"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/mdlayher/vsock"
	"github.com/mdlayher/vsock/internal/vsutil"
	"golang.org/x/sys/unix"
)

func TestIntegrationListenerUnblockAcceptTimeout(t *testing.T) {
	l, done := newListener(t)
	defer done()

	if err := l.SetDeadline(time.Now().Add(100 * time.Millisecond)); err != nil {
		t.Fatalf("failed to set listener deadline: %v", err)
	}

	_, err := l.Accept()
	if err == nil {
		t.Fatal("expected an error, but none occurred")
	}

	if nerr, ok := err.(net.Error); !ok || (ok && !nerr.Temporary()) {
		t.Errorf("expected temporary network error, but got: %#v", err)
	}
}

func TestIntegrationConnSyscallConn(t *testing.T) {
	if vsutil.IsHypervisor(t) {
		t.Skip("skipping, this test must be run in a guest")
	}

	mp := makeVSockPipe()

	c, _, stop, err := mp()
	if err != nil {
		t.Fatalf("failed to make pipe: %v", err)
	}
	defer stop()

	rc, err := c.(*vsock.Conn).SyscallConn()
	if err != nil {
		t.Fatalf("failed to syscallconn: %v", err)
	}

	// Greatly reduce the size of the socket buffer.
	const (
		size uint64 = 64
		name        = unix.SO_VM_SOCKETS_BUFFER_MAX_SIZE
	)

	err = rc.Control(func(fd uintptr) {
		err := unix.SetsockoptUint64(int(fd), unix.AF_VSOCK, name, size)
		if err != nil {
			t.Fatalf("failed to setsockopt: %v", err)
		}

		out, err := unix.GetsockoptUint64(int(fd), unix.AF_VSOCK, name)
		if err != nil {
			t.Fatalf("failed to getsockopt: %v", err)
		}

		if diff := cmp.Diff(size, out); diff != "" {
			t.Fatalf("unexpected socket buffer size (-want +got):\n%s", diff)
		}
	})
	if err != nil {
		t.Fatalf("failed to control: %v", err)
	}
}

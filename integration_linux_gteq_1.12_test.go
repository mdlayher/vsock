//+build go1.12,linux

package vsock_test

import (
	"fmt"
	"io"
	"net"
	"sync"
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

func TestIntegrationConnShutdown(t *testing.T) {
	vsutil.SkipHostIntegration(t)

	// This functionality is proposed for inclusion in x/net/nettest, and should
	// be removed from here if the proposal is accepted. See:
	// https://github.com/golang/go/issues/31033.

	timer := time.AfterFunc(10*time.Second, func() {
		panic("test took too long")
	})
	defer timer.Stop()

	mp := makeVSockPipe()

	c1, c2, stop, err := mp()
	if err != nil {
		t.Fatalf("failed to make pipe: %v", err)
	}
	defer stop()

	vc1, vc2 := c1.(*vsock.Conn), c2.(*vsock.Conn)

	var wg sync.WaitGroup
	wg.Add(1)
	defer wg.Wait()

	// Perform CloseRead/CloseWrite in lock-step.
	var (
		readClosed  = make(chan struct{}, 0)
		writeClosed = make(chan struct{}, 0)
	)

	go func() {
		defer wg.Done()

		b := make([]byte, 8)
		if _, err := io.ReadFull(vc2, b); err != nil {
			panicf("failed to vc2.Read: %v", err)
		}

		if err := vc2.CloseRead(); err != nil {
			panicf("failed to vc2.CloseRead: %v", err)
		}
		close(readClosed)

		// Any further read should return io.EOF.
		<-writeClosed
		if _, err := io.ReadFull(vc2, b); err != io.EOF {
			panicf("expected vc2.Read EOF, but got: %v", err)
		}

		if err := vc2.CloseWrite(); err != nil {
			panicf("failed to vc2.CloseWrite: %v", err)
		}
	}()

	b := make([]byte, 8)
	if _, err := vc1.Write(b); err != nil {
		t.Fatalf("failed to vc1.Write: %v", err)
	}

	// Any write to a read-closed connection should return EPIPE.
	<-readClosed
	if _, err := vc1.Write(b); !isBrokenPipe(err) {
		t.Fatalf("expected vc1.Write broken pipe, but got: %v", err)
	}

	if err := vc1.CloseWrite(); err != nil {
		t.Fatalf("failed to vc1.CloseWrite: %v", err)
	}
	close(writeClosed)

	// Any further read should return io.EOF after the other end write-closes.
	if _, err := io.ReadFull(vc1, b); err != io.EOF {
		panicf("expected vc1.Read EOF, but got: %v", err)
	}
}

func TestIntegrationConnSyscallConn(t *testing.T) {
	vsutil.SkipHostIntegration(t)

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

func isBrokenPipe(err error) bool {
	if err == nil {
		return false
	}

	nerr, ok := err.(*net.OpError)
	if !ok {
		return false
	}

	return nerr.Err == unix.EPIPE
}

func panicf(format string, a ...interface{}) {
	panic(fmt.Sprintf(format, a...))
}

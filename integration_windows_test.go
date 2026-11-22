package vsock_test

import (
	"context"
	"errors"
	"io"
	"net"
	"os"
	"testing"
	"time"

	"github.com/mdlayher/vsock"
	"github.com/mdlayher/vsock/internal/vsutil"
	"golang.org/x/net/nettest"
	"golang.org/x/sys/windows"
)

func TestIntegrationContextID(t *testing.T) {
	cid, err := vsock.ContextID()
	if err != nil {
		vsutil.SkipDeviceError(t, err)

		t.Fatalf("failed to retrieve guest's context ID: %v", err)
	}

	t.Logf("guest context ID: %d", cid)

	// Guests should always have a context ID of 3 or more, since
	// 0-2 are invalid or reserved.
	if cid < 3 {
		t.Fatalf("unexpected guest context ID: %d", cid)
	}
}

func TestIntegrationListenerAddr(t *testing.T) {
	l := newListener(t)

	addr := l.Addr().(*vsock.Addr)
	if addr.Port == 0 || addr.Port == ^uint32(0) {
		t.Fatalf("unexpected listener port: %d", addr.Port)
	}
}

func TestIntegrationListenerUnblockAcceptTimeout(t *testing.T) {
	l := newListener(t)

	err := l.SetDeadline(time.Now().Add(100 * time.Millisecond))
	if err != nil {
		t.Fatalf("failed to set listener deadline: %v", err)
	}

	start := time.Now()

	_, err = l.Accept()
	if !errors.Is(err, os.ErrDeadlineExceeded) {
		t.Fatalf("expected a timeout error, but got: %v", err)
	}

	elapsed := time.Since(start)
	if elapsed > 500*time.Millisecond {
		t.Fatalf("accept took too long to time out: %s", elapsed)
	}
}

func TestIntegrationListenerUnblockAcceptSetDeadline(t *testing.T) {
	l := newListener(t)

	// Unblock a pending Accept by moving the deadline.
	time.AfterFunc(100*time.Millisecond, func() {
		_ = l.SetDeadline(time.Now())
	})

	start := time.Now()

	_, err := l.Accept()
	if !errors.Is(err, os.ErrDeadlineExceeded) {
		t.Fatalf("expected a timeout error, but got: %v", err)
	}

	elapsed := time.Since(start)
	if elapsed > 500*time.Millisecond {
		t.Fatalf("accept took too long to time out: %s", elapsed)
	}
}

func TestIntegrationListenerUnblockAcceptClose(t *testing.T) {
	l := newListener(t)

	time.AfterFunc(100*time.Millisecond, func() {
		_ = l.Close()
	})

	start := time.Now()

	_, err := l.Accept()
	if !errors.Is(err, net.ErrClosed) {
		t.Fatalf("expected a closed error, but got: %v", err)
	}

	elapsed := time.Since(start)
	if elapsed > 500*time.Millisecond {
		t.Fatalf("accept took too long to return: %s", elapsed)
	}

	_, err = l.Accept()
	if !errors.Is(err, net.ErrClosed) {
		t.Fatalf("expected a closed error, but got: %v", err)
	}
}

func TestIntegrationConnUnblockReadSetDeadline(t *testing.T) {
	c1, _ := newPipe(t)

	// Unblock a pending Read by moving the deadline.
	time.AfterFunc(100*time.Millisecond, func() {
		_ = c1.SetReadDeadline(time.Now())
	})

	start := time.Now()

	_, err := c1.Read(make([]byte, 8))
	if !errors.Is(err, os.ErrDeadlineExceeded) {
		t.Fatalf("expected a timeout error, but got: %v", err)
	}

	elapsed := time.Since(start)
	if elapsed > 500*time.Millisecond {
		t.Fatalf("read took too long to time out: %s", elapsed)
	}
}

func TestIntegrationConnUnblockReadClose(t *testing.T) {
	c1, _ := newPipe(t)

	time.AfterFunc(100*time.Millisecond, func() {
		_ = c1.Close()
	})

	start := time.Now()

	_, err := c1.Read(make([]byte, 8))
	if !errors.Is(err, net.ErrClosed) {
		t.Fatalf("expected a closed error, but got: %v", err)
	}

	elapsed := time.Since(start)
	if elapsed > 500*time.Millisecond {
		t.Fatalf("read took too long to return: %s", elapsed)
	}
}

func TestIntegrationConnClosed(t *testing.T) {
	c1, _ := newPipe(t)

	err := c1.Close()
	if err != nil {
		t.Fatalf("failed to close connection: %v", err)
	}

	_, err = c1.Read(make([]byte, 8))
	if !errors.Is(err, net.ErrClosed) {
		t.Fatalf("expected a closed error from Read, but got: %v", err)
	}

	_, err = c1.Write(make([]byte, 8))
	if !errors.Is(err, net.ErrClosed) {
		t.Fatalf("expected a closed error from Write, but got: %v", err)
	}

	err = c1.CloseRead()
	if !errors.Is(err, net.ErrClosed) {
		t.Fatalf("expected a closed error from CloseRead, but got: %v", err)
	}

	err = c1.CloseWrite()
	if !errors.Is(err, net.ErrClosed) {
		t.Fatalf("expected a closed error from CloseWrite, but got: %v", err)
	}
}

func TestIntegrationConnShutdown(t *testing.T) {
	c1, c2 := newPipe(t)

	b := make([]byte, 8)

	_, err := c1.Write(b)
	if err != nil {
		t.Fatalf("failed to c1.Write: %v", err)
	}

	_, err = io.ReadFull(c2, b)
	if err != nil {
		t.Fatalf("failed to c2.Read: %v", err)
	}

	// Any read should return io.EOF after the other end write-closes.
	err = c1.CloseWrite()
	if err != nil {
		t.Fatalf("failed to c1.CloseWrite: %v", err)
	}

	_, err = io.ReadFull(c2, b)
	if err != io.EOF {
		t.Fatalf("expected c2.Read EOF, but got: %v", err)
	}

	// The other direction must still be usable.
	_, err = c2.Write(b)
	if err != nil {
		t.Fatalf("failed to c2.Write: %v", err)
	}

	_, err = io.ReadFull(c1, b)
	if err != nil {
		t.Fatalf("failed to c1.Read: %v", err)
	}

	// Any read should return io.EOF after a local read-close.
	err = c1.CloseRead()
	if err != nil {
		t.Fatalf("failed to c1.CloseRead: %v", err)
	}

	_, err = io.ReadFull(c1, b)
	if err != io.EOF {
		t.Fatalf("expected c1.Read EOF, but got: %v", err)
	}
}

func TestIntegrationConnDialNoListener(t *testing.T) {
	l := newListener(t)
	addr := l.Addr().(*vsock.Addr)

	// Dial out to a vsock listener which does not exist, and expect an
	// error rather than hanging.
	start := time.Now()

	_, err := vsock.Dial(addr.ContextID, ^uint32(0)-1, nil)
	if err == nil {
		t.Fatal("dial succeeded, but should not have")
	}

	t.Logf("dial error after %s: %v", time.Since(start), err)

	got, ok := err.(*net.OpError)
	if !ok {
		t.Fatalf("expected *net.OpError, but got %T", err)
	}

	if got.Op != "dial" || got.Net != "vsock" {
		t.Fatalf("unexpected error: %#v", got)
	}
}

func TestIntegrationConnDialContextCanceled(t *testing.T) {
	l := newListener(t)
	addr := l.Addr().(*vsock.Addr)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := vsock.DialContext(ctx, addr.ContextID, addr.Port, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected a canceled error, but got: %v", err)
	}
}

func TestIntegrationConnDialContextTimeout(t *testing.T) {
	l := newListener(t)
	addr := l.Addr().(*vsock.Addr)

	// Without local support, connecting to the guest itself goes unanswered.
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	start := time.Now()

	c, err := vsock.DialContext(ctx, addr.ContextID, addr.Port, nil)
	if err == nil {
		_ = c.Close()
		t.Skip("skipping, connection to the guest itself succeeded")
	}

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected a deadline exceeded error, but got: %v", err)
	}

	elapsed := time.Since(start)
	if elapsed > time.Second {
		t.Fatalf("dial took too long to return: %s", elapsed)
	}
}

func TestIntegrationConnDialContextEstablished(t *testing.T) {
	skipNoLocal(t)

	l, err := vsock.ListenContextID(vsock.Local, 0, nil)
	if err != nil {
		t.Fatalf("failed to create vsock listener: %v", err)
	}
	defer l.Close()

	addr := l.Addr().(*vsock.Addr)

	// The context must not affect an established connection.
	ctx, cancel := context.WithCancel(context.Background())

	c1, err := vsock.DialContext(ctx, addr.ContextID, addr.Port, nil)
	if err != nil {
		t.Fatalf("failed to dial: %v", err)
	}
	defer c1.Close()

	c2, err := l.Accept()
	if err != nil {
		t.Fatalf("failed to accept: %v", err)
	}
	defer c2.Close()

	cancel()

	b := make([]byte, 8)

	_, err = c1.Write(b)
	if err != nil {
		t.Fatalf("failed to c1.Write: %v", err)
	}

	_, err = io.ReadFull(c2, b)
	if err != nil {
		t.Fatalf("failed to c2.Read: %v", err)
	}
}

func TestIntegrationConnLargeTransfer(t *testing.T) {
	c1, c2 := newPipe(t)

	// Exceed the socket buffers to exercise partial writes.
	want := make([]byte, 16<<20)
	for i := range want {
		want[i] = byte(i)
	}

	errC := make(chan error, 1)
	go func() {
		_, err := c1.Write(want)
		if err == nil {
			err = c1.CloseWrite()
		}

		errC <- err
	}()

	got, err := io.ReadAll(c2)
	if err != nil {
		t.Fatalf("failed to read: %v", err)
	}

	err = <-errC
	if err != nil {
		t.Fatalf("failed to write: %v", err)
	}

	if string(want) != string(got) {
		t.Fatalf("unexpected data: read %d bytes, want %d", len(got), len(want))
	}
}

func TestIntegrationNettestTestConn(t *testing.T) {
	skipNoLocal(t)

	nettest.TestConn(t, makePipe)
}

// newListener creates a listener, skipping the test if VM sockets are
// unavailable.
func newListener(t *testing.T) *vsock.Listener {
	t.Helper()

	skipNoDriver(t)

	l, err := vsock.Listen(0, nil)
	if err != nil {
		vsutil.SkipDeviceError(t, err)

		t.Fatalf("failed to create vsock listener: %v", err)
	}

	t.Cleanup(func() { _ = l.Close() })

	return l
}

// newPipe creates a pair of connected connections.
func newPipe(t *testing.T) (*vsock.Conn, *vsock.Conn) {
	t.Helper()

	skipNoLocal(t)

	c1, c2, stop, err := makePipe()
	if err != nil {
		t.Fatalf("failed to make pipe: %v", err)
	}

	t.Cleanup(stop)

	return c1.(*vsock.Conn), c2.(*vsock.Conn)
}

// skipNoDriver skips the test if the viosock driver isn't installed.
func skipNoDriver(t *testing.T) {
	t.Helper()

	_, err := vsock.ContextID()
	if err != nil {
		vsutil.SkipDeviceError(t, err)

		t.Fatalf("failed to retrieve guest's context ID: %v", err)
	}
}

// skipNoLocal skips the test if connections can't be made within the guest.
func skipNoLocal(t *testing.T) {
	t.Helper()

	skipNoDriver(t)

	l, err := vsock.ListenContextID(vsock.Local, 0, nil)
	if err != nil {
		vsutil.SkipDeviceError(t, err)

		if errors.Is(err, windows.WSAEADDRNOTAVAIL) {
			t.Skipf("skipping, viosock driver is too old to support local binds: %v", err)
		}

		t.Fatalf("failed to create vsock listener: %v", err)
	}

	_ = l.Close()
}

// makePipe is a nettest.MakePipe for VM sockets.
func makePipe() (net.Conn, net.Conn, func(), error) {
	l, err := vsock.ListenContextID(vsock.Local, 0, nil)
	if err != nil {
		return nil, nil, nil, err
	}
	defer l.Close()

	addr := l.Addr().(*vsock.Addr)

	c1, err := vsock.Dial(addr.ContextID, addr.Port, nil)
	if err != nil {
		return nil, nil, nil, err
	}

	c2, err := l.Accept()
	if err != nil {
		_ = c1.Close()
		return nil, nil, nil, err
	}

	stop := func() {
		_ = c1.Close()
		_ = c2.Close()
	}

	return c1, c2, stop, nil
}

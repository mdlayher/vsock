//go:build linux
// +build linux

package vsock_test

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"os"
	"regexp"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/mdlayher/vsock"
	"github.com/mdlayher/vsock/internal/vsutil"
	"golang.org/x/net/nettest"
	"golang.org/x/sync/errgroup"
	"golang.org/x/sys/unix"
)

func TestIntegrationContextIDGuest(t *testing.T) {
	if vsutil.IsHypervisor(t) {
		t.Skip("skipping, machine is not a guest")
	}

	cid, err := vsock.ContextID()
	if err != nil {
		t.Fatalf("failed to retrieve guest's context ID: %v", err)
	}

	t.Logf("guest context ID: %d", cid)

	// Guests should always have a context ID of 3 or more, since
	// 0-2 are invalid or reserved.
	if cid < 3 {
		t.Fatalf("unexpected guest context ID: %d", cid)
	}
}

func TestIntegrationContextIDHost(t *testing.T) {
	if !vsutil.IsHypervisor(t) {
		t.Skip("skipping, machine is not a hypervisor")
	}

	cid, err := vsock.ContextID()
	if err != nil {
		t.Fatalf("failed to retrieve host's context ID: %v", err)
	}

	t.Logf("host context ID: %d", cid)

	if want, got := uint32(vsock.Host), cid; want != got {
		t.Fatalf("unexpected host context ID:\n- want: %d\n-  got: %d",
			want, got)
	}
}

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

	if nerr, ok := err.(net.Error); !ok || (ok && !nerr.Timeout()) {
		t.Errorf("expected timeout network error, but got: %#v", err)
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
		readClosed  = make(chan struct{})
		writeClosed = make(chan struct{})
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
	//
	// TODO(mdlayher): this test was flappy until err != nil check was added;
	// sometimes it returns EPIPE and sometimes it does not. Check this.
	<-readClosed
	if _, err := vc1.Write(b); err != nil && !isBrokenPipe(err) {
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

func TestIntegrationConnDialNoListener(t *testing.T) {
	// Dial out to vsock listeners which do not exist, and expect an immediate
	// error rather than hanging. This mostly relies on changes to the
	// underlying socket library, but we test it anyway to lock things in.
	//
	// See: https://github.com/mdlayher/vsock/issues/47.
	const max = math.MaxUint32
	for _, port := range []uint32{max - 2, max - 1, max} {
		_, err := vsock.Dial(vsock.Local, port, nil)
		if err == nil {
			t.Fatal("dial succeeded, but should not have")
		}

		got, ok := err.(*net.OpError)
		if !ok {
			t.Fatalf("expected *net.OpError, but got %T", err)
		}

		// Expect one of ECONNRESET or ENODEV depending on the kernel.
		switch {
		case errors.Is(got.Err, unix.ECONNRESET), errors.Is(got.Err, unix.ENODEV):
			// OK.
		default:
			t.Fatalf("unexpected syscall error: %v", got.Err)
		}

		// Zero out the error comparison.
		got.Err = nil

		want := &net.OpError{
			Op:   "dial",
			Net:  "vsock",
			Addr: &vsock.Addr{ContextID: vsock.Local, Port: port},
		}

		if diff := cmp.Diff(want, err); diff != "" {
			t.Errorf("unexpected error (-want +got):\n%s", diff)
		}
	}
}

func TestIntegrationFileListenerOK(t *testing.T) {
	// Use raw system calls to set up the socket for FileListener. Although the
	// socket library does the heavy lifting, we want to verify that this also
	// works specifically for AF_VSOCK.
	fd, err := unix.Socket(unix.AF_VSOCK, unix.SOCK_STREAM, 0)
	if err != nil {
		t.Fatalf("failed to open socket: %v", err)
	}

	// Bind to local, any available port.
	err = unix.Bind(fd, &unix.SockaddrVM{
		CID:  unix.VMADDR_CID_LOCAL,
		Port: unix.VMADDR_PORT_ANY,
	})
	if err != nil {
		// Same problem with GitHub actions kernel, investigate.
		switch err {
		case unix.EADDRNOTAVAIL:
			skipOldKernel(t)
		default:
			t.Fatalf("failed to bind: %v", err)
		}
	}

	if err := unix.Listen(fd, unix.SOMAXCONN); err != nil {
		t.Fatalf("failed to listen: %v", err)
	}

	// The socket should be ready, create a blocking file which is ready to be
	// passed into FileListener.
	f := os.NewFile(uintptr(fd), "vsock-listener")
	defer f.Close()

	l, err := vsock.FileListener(f)
	if err != nil {
		t.Fatalf("failed to open file listener: %v", err)
	}
	defer l.Close()

	// To exercise the listener, attempt to accept and then immediately close a
	// single vsock connection. Dial to the listener from the main goroutine and
	// wait for everything to finish.
	var eg errgroup.Group
	eg.Go(func() error {
		c, err := l.Accept()
		if err != nil {
			return fmt.Errorf("failed to accept: %v", err)
		}

		_ = c.Close()
		return nil
	})

	addr := l.Addr().(*vsock.Addr)
	c, err := vsock.Dial(addr.ContextID, addr.Port, nil)
	if err != nil {
		t.Fatalf("failed to dial listener: %v", err)
	}
	_ = c.Close()

	if err := eg.Wait(); err != nil {
		t.Fatalf("failed to wait for listener goroutine: %v", err)
	}
}

func TestIntegrationFileListenerInvalid(t *testing.T) {
	// Same idea as the previous test, but intentionally create a TCP socket
	// instead of a vsock so we can verify the library rejects the socket.
	fd, err := unix.Socket(unix.AF_INET6, unix.SOCK_STREAM, 0)
	if err != nil {
		t.Fatalf("failed to open socket: %v", err)
	}

	// Bind to any address.
	if err := unix.Bind(fd, &unix.SockaddrInet6{}); err != nil {
		t.Fatalf("failed to bind: %v", err)
	}

	if err := unix.Listen(fd, unix.SOMAXCONN); err != nil {
		t.Fatalf("failed to listen: %v", err)
	}

	// The library should reject this file for having the wrong address family.
	f := os.NewFile(uintptr(fd), "tcpv6-listener")
	defer f.Close()

	_, got := vsock.FileListener(f)

	want := &net.OpError{
		Op:  "listen",
		Net: "vsock",
		Err: os.NewSyscallError("listen", unix.EINVAL),
	}

	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatalf("unexpected error (-want +got):\n%s", diff)
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

func TestIntegrationNettestTestConn(t *testing.T) {
	vsutil.SkipHostIntegration(t)

	nettest.TestConn(t, makeVSockPipe())
}

var cidRe = regexp.MustCompile(`\S+\((\d+)\)`)

func TestIntegrationNettestTestListener(t *testing.T) {
	vsutil.SkipHostIntegration(t)

	// This test uses the nettest.TestListener API which is being built in:
	// https://go-review.googlesource.com/c/net/+/123056.
	//
	// TODO(mdlayher): stop skipping this test once that CL lands.

	mos := func() (ln net.Listener, dial func(string, string) (net.Conn, error), stop func(), err error) {
		l, err := vsock.ListenContextID(vsock.Local, 0, nil)
		if err != nil {
			return nil, nil, nil, err
		}

		stop = func() {
			// TODO(mdlayher): cancel context if we use vsock.DialContext.
			_ = l.Close()
		}

		dial = func(_, addr string) (net.Conn, error) {
			host, sport, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, err
			}

			// Extract the CID value from the surrounding text.
			scid := cidRe.FindStringSubmatch(host)
			cid, err := strconv.Atoi(scid[1])
			if err != nil {
				return nil, err
			}

			port, err := strconv.Atoi(sport)
			if err != nil {
				return nil, err
			}

			return vsock.Dial(uint32(cid), uint32(port), nil)
		}

		return l, dial, stop, nil
	}

	_ = mos
	t.Skip("skipping, enable once https://go-review.googlesource.com/c/net/+/123056 is merged")
	// nettest.TestListener(t, mos)
}

func newListener(t *testing.T) (*vsock.Listener, func()) {
	t.Helper()

	timer := time.AfterFunc(10*time.Second, func() {
		panic("test took too long")
	})

	// Bind to Local for all integration tests to avoid the need to run a
	// hypervisor and VM setup.
	l, err := vsock.ListenContextID(vsock.Local, 0, nil)
	if err != nil {
		vsutil.SkipDeviceError(t, err)

		// Unwrap net.OpError + os.SyscallError if needed.
		// TODO(mdlayher): errors.Unwrap in Go 1.13.
		nerr, ok := err.(*net.OpError)
		if !ok {
			t.Fatalf("failed to create vsock listener: %v", err)
		}
		serr, ok := nerr.Err.(*os.SyscallError)
		if !ok {
			t.Fatalf("unexpected inner error for *net.OpError: %#v", nerr.Err)
		}

		switch serr.Err {
		case unix.EADDRNOTAVAIL:
			skipOldKernel(t)
		default:
			t.Fatalf("unexpected vsock listener system call error: %v", err)
		}
	}

	return l, func() {
		// Clean up the timer and this listener.
		timer.Stop()
		_ = l.Close()
	}
}

func skipOldKernel(t *testing.T) {
	t.Helper()

	// The kernel in use is to old to support Local binds, so this
	// test must be skipped. Print an informative message.
	var utsname unix.Utsname
	if err := unix.Uname(&utsname); err != nil {
		t.Fatalf("failed to get uname: %v", err)
	}

	t.Skipf("skipping, kernel %s is too old to support AF_VSOCK local binds",
		string(bytes.TrimRight(utsname.Release[:], "\x00")))
}

func makeVSockPipe() nettest.MakePipe {
	return makeLocalPipe(
		func() (net.Listener, error) { return vsock.ListenContextID(vsock.Local, 0, nil) },
		func(addr net.Addr) (net.Conn, error) {
			// ContextID will always be Local.
			a := addr.(*vsock.Addr)
			return vsock.Dial(a.ContextID, a.Port, nil)
		},
	)
}

// makeLocalPipe produces a nettest.MakePipe function using the input functions
// to configure a net.Listener and point a net.Conn at the listener.
//
// This function is proposed for inclusion in x/net/nettest, and should be
// removed from here if the proposal is accepted. See:
// https://github.com/golang/go/issues/30984.
func makeLocalPipe(
	listen func() (net.Listener, error),
	dial func(addr net.Addr) (net.Conn, error),
) nettest.MakePipe {
	if listen == nil {
		panic("nil listen function passed to makeLocalPipe")
	}

	if dial == nil {
		dial = func(addr net.Addr) (net.Conn, error) {
			return net.Dial(addr.Network(), addr.String())
		}
	}

	// The majority of this code is taken from golang.org/x/net/nettest:
	// https://go.googlesource.com/net/+/9e4ed9723b84cb6661bb04e4104f7bfb3ff5d016/nettest/conntest_test.go.
	//
	// Copyright 2016 The Go Authors. All rights reserved.

	return func() (c1, c2 net.Conn, stop func(), err error) {
		ln, err := listen()
		if err != nil {
			return nil, nil, nil, fmt.Errorf("failed to create local listener: %v", err)
		}

		// Start a connection between two endpoints.
		var err1, err2 error
		done := make(chan bool)
		go func() {
			c2, err2 = ln.Accept()
			close(done)
		}()
		c1, err1 = dial(ln.Addr())
		<-done

		stop = func() {
			if err1 == nil {
				c1.Close()
			}
			if err2 == nil {
				c2.Close()
			}
			ln.Close()
			switch ln.Addr().Network() {
			case "unix", "unixpacket":
				os.Remove(ln.Addr().String())
			}
		}

		switch {
		case err1 != nil:
			stop()
			return nil, nil, nil, err1
		case err2 != nil:
			stop()
			return nil, nil, nil, err2
		default:
			return c1, c2, stop, nil
		}
	}
}

func panicf(format string, a ...interface{}) {
	panic(fmt.Sprintf(format, a...))
}

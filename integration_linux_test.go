//go:build linux
// +build linux

package vsock_test

import (
	"fmt"
	"net"
	"os"
	"regexp"
	"strconv"
	"testing"
	"time"

	"github.com/mdlayher/vsock"
	"github.com/mdlayher/vsock/internal/vsutil"
	"golang.org/x/net/nettest"
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
		l, err := vsock.Listen(vsock.Loopback, 0)
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

			return vsock.Dial(uint32(cid), uint32(port))
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

	l, err := vsock.Listen(0x01, 0)
	if err != nil {
		vsutil.SkipDeviceError(t, err)

		t.Fatalf("failed to create vsock listener: %v", err)
	}

	return l, func() {
		// Clean up the timer and this listener.
		timer.Stop()
		_ = l.Close()
	}
}

func makeVSockPipe() nettest.MakePipe {
	return makeLocalPipe(
		func() (net.Listener, error) { return vsock.Listen(0x01, 0) },
		func(addr net.Addr) (net.Conn, error) {
			a := addr.(*vsock.Addr)
			return vsock.Dial(a.ContextID, a.Port)
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

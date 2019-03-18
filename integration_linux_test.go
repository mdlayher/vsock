//+build linux

package vsock_test

import (
	"net"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mdlayher/vsock"
	"github.com/mdlayher/vsock/internal/vsutil"
)

func TestIntegrationListenerUnblockAcceptAfterClose(t *testing.T) {
	l, done := newListener(t)
	defer done()

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()

		t.Log("start accept")
		_, err := vsutil.Accept(l, 10*time.Second)
		t.Log("after accept")

		if nerr, ok := err.(net.Error); ok && nerr.Temporary() {
			t.Errorf("expected permanent error, but got temporary one: %v", err)
		}

		// Go1.11:
		if strings.Contains(err.Error(), "bad file descriptor") {
			// All is well, the file descriptor was closed.
			return
		}

		// Go 1.12+:
		// TODO(mdlayher): wrap string error in net.OpError or similar.
		if !strings.Contains(err.Error(), "use of closed file") {
			t.Errorf("unexpected accept error: %v", err)
		}
	}()

	time.Sleep(100 * time.Millisecond)

	if err := l.Close(); err != nil {
		t.Fatalf("failed to close listener: %v", err)
	}

	doneC := make(chan struct{})
	go func() {
		wg.Wait()
		close(doneC)
	}()

	select {
	case <-doneC:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting accept to unblock")
	}
}

func newListener(t *testing.T) (*vsock.Listener, func()) {
	t.Helper()

	timer := time.AfterFunc(10*time.Second, func() {
		panic("test took too long")
	})

	l, err := vsock.Listen(0)
	if err == nil {
		return l, func() {
			// Clean up the timer and this listener.
			timer.Stop()
			_ = l.Close()
		}
	}

	if os.IsNotExist(err) {
		t.Skipf("skipping, vsock device does not exist (try: 'modprobe vhost_vsock'): %v", err)
	}
	if os.IsPermission(err) {
		t.Skipf("skipping, permission denied: %v", err)
	}

	t.Fatalf("failed to create vsock listener: %v", err)
	panic("unreachable")
}

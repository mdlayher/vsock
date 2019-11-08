//+build go1.11,linux

package vsock_test

import (
	"net"
	"strings"
	"sync"
	"testing"
	"time"

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

		nerr, ok := err.(*net.OpError)
		if !ok {
			t.Errorf("expected a net.OpError, but got: %#v", err)
		}

		if nerr.Temporary() {
			t.Errorf("expected permanent error, but got temporary one: %v", err)
		}

		// We mimic what net.TCPConn does and return an error with the same
		// string as internal/poll, so string matching is the best we can do
		// for now.
		if !strings.Contains(nerr.Err.Error(), "use of closed") {
			t.Errorf("expected close network connection error, but got: %v", nerr.Err)
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

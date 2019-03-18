//+build go1.12,linux

package vsock_test

import (
	"net"
	"testing"
	"time"
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

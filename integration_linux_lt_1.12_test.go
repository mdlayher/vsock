//go:build !go1.12 && linux
// +build !go1.12,linux

package vsock_test

import (
	"testing"
	"time"

	"github.com/mdlayher/vsock"
	"github.com/mdlayher/vsock/internal/vsutil"
)

func TestIntegrationListenerSetDeadlineError(t *testing.T) {
	l, done := newListener(t)
	defer done()

	err := l.SetDeadline(time.Time{})
	if err == nil {
		t.Fatal("expected an error, but none occurred")
	}

	t.Logf("OK error: %v", err)
}

func TestIntegrationConnSyscallConnError(t *testing.T) {
	vsutil.SkipHostIntegration(t)

	mp := makeVSockPipe()

	c, _, stop, err := mp()
	if err != nil {
		t.Fatalf("failed to make pipe: %v", err)
	}
	defer stop()

	_, err = c.(*vsock.Conn).SyscallConn()
	if err == nil {
		t.Fatal("expected an error, but none occurred")
	}

	t.Logf("OK error: %v", err)
}

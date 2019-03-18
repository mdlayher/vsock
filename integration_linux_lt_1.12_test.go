//+build !go1.12,linux

package vsock_test

import (
	"testing"
	"time"
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

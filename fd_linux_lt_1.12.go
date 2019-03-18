//+build !go1.12,linux

package vsock

import (
	"fmt"
	"runtime"
	"time"

	"golang.org/x/sys/unix"
)

func (lfd *sysListenFD) accept4(flags int) (int, unix.Sockaddr, error) {
	// In Go 1.11, accept on the raw file descriptor directly, because lfd.f
	// may be attached to the runtime network poller, forcing this call to block
	// even if Close is called.
	return unix.Accept4(lfd.fd, flags)
}

func (lfd *sysListenFD) setDeadline(t time.Time) error {
	// Listener deadlines won't work as expected in this version of Go, so
	// return an early error.
	return fmt.Errorf("vsock: listener deadlines not supported on %s", runtime.Version())
}

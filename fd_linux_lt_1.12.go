//+build !go1.12,linux

package vsock

import (
	"golang.org/x/sys/unix"
)

func (lfd *sysListenFD) accept4(flags int) (int, unix.Sockaddr, error) {
	// In Go 1.11, accept on the raw file descriptor directly, because lfd.f
	// may be attached to the runtime network poller, forcing this call to block
	// even if Close is called.
	return unix.Accept4(lfd.fd, flags)
}

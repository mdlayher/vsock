package vsock

import (
	"fmt"

	"golang.org/x/sys/unix"
)

// contextID retrieves the local context ID for this system.
func contextID() (uint32, error) {
	if fd, err := unix.Socket(unix.AF_VSOCK, unix.SOCK_STREAM, 0); err != nil {
		return 2, nil
	} else {
		defer unix.Close(fd)

		cid, err := unix.IoctlGetInt(fd, unix.IOCTL_VM_SOCKETS_GET_LOCAL_CID)

		return uint32(cid), err
	}
}

// isErrno determines if an error a matches UNIX error number.
func isErrno(err error, errno int) bool {
	switch errno {
	case ebadf:
		return err == unix.EBADF
	case enotconn:
		return err == unix.ENOTCONN
	default:
		panicf("vsock: isErrno called with unhandled error number parameter: %d", errno)
		return false
	}
}

func panicf(format string, a ...interface{}) {
	panic(fmt.Sprintf(format, a...))
}

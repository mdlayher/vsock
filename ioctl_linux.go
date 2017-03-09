package vsock

import (
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

const (
	// ioctlGetLocalCID is an ioctl value that retrieves the local context ID
	// from /dev/vsock.
	// TODO(mdlayher): this is probably linux/amd64 specific, but I'm unsure
	// how to make it portable across architectures.
	ioctlGetLocalCID = 0x7b9

	// devVsock is the location of /dev/vsock.
	devVsock = "/dev/vsock"
)

// A fs is an interface over the filesystem and ioctl, to enable testing.
type fs interface {
	Open(name string) (*os.File, error)
	Ioctl(fd uintptr, request int, argp uintptr) error
}

// localContextID retrieves the local context ID for this system, using the
// methods from fs.
func localContextID(fs fs) (uint32, error) {
	f, err := fs.Open(devVsock)
	if err != nil {
		// If /dev/vsock doesn't exist, assume this is the hypervisor.
		// Unfortunately, this also means that machines that don't support
		// VM sockets will hit this case.
		//
		// TODO(mdlayher): attempt to differentiate VM sockets being unsupported
		// and /dev/vsock just not being available on a hypervisor that does support
		// them.
		if os.IsNotExist(err) {
			return ContextIDHost, nil
		}

		return 0, err
	}
	defer f.Close()

	// Retrieve the context ID of this machine from /dev/vsock.
	var cid uint32
	err = fs.Ioctl(f.Fd(), ioctlGetLocalCID, uintptr(unsafe.Pointer(&cid)))
	if err != nil {
		return 0, err
	}

	return cid, nil
}

// A sysFS is the system call implementation of fs.
type sysFS struct{}

func (sysFS) Open(name string) (*os.File, error) { return os.Open(name) }
func (sysFS) Ioctl(fd uintptr, request int, argp uintptr) error {
	_, _, errno := syscall.Syscall(
		syscall.SYS_IOCTL,
		fd,
		uintptr(request),
		argp,
	)
	if errno != 0 {
		return os.NewSyscallError("ioctl", fmt.Errorf("%d", int(errno)))
	}

	return nil
}

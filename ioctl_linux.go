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

	// devVsock is the location of /dev/vsock.  It is exposed on both the
	// hypervisor and on virtual machines.
	devVsock = "/dev/vsock"
)

// A fs is an interface over the filesystem and ioctl, to enable testing.
type fs interface {
	Open(name string) (*os.File, error)
	Ioctl(fd uintptr, request int, argp uintptr) error
}

// localContextID retrieves the local context ID for this system, using the
// methods from fs.  The context ID is stored in cid for later use.
//
// This method uses this signature to enable easier testing without unsafe
// usage of unsafe.Pointer.
func localContextID(fs fs, cid *uint32) error {
	f, err := fs.Open(devVsock)
	if err != nil {
		return err
	}
	defer f.Close()

	// Retrieve the context ID of this machine from /dev/vsock.
	return fs.Ioctl(f.Fd(), ioctlGetLocalCID, uintptr(unsafe.Pointer(cid)))
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

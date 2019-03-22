// +build linux

package vsock

import (
	"os"
	"testing"
	"unsafe"

	"golang.org/x/sys/unix"
)

func Test_localContextIDGuest(t *testing.T) {
	const (
		fd        uintptr = 10
		contextID uint32  = 5
	)

	// Since it isn't safe to manipulate the argument pointer with
	// ioctl, we check that the ioctl performs its commands
	// on the appropriate file descriptor and request number, and
	// then use a map to emulate the ioctl setting the context ID
	// into a *uint32.
	//
	// Thanks to @zeebo from Gophers Slack for this idea.
	var cid uint32
	cfds := map[uintptr]*uint32{
		fd: &cid,
	}

	ioctl := func(ioctlFD uintptr, request int, _ unsafe.Pointer) error {
		if want, got := fd, ioctlFD; want != got {
			t.Fatalf("unexpected file descriptor for ioctl:\n- want: %d\n-  got: %d",
				want, got)
		}

		if want, got := unix.IOCTL_VM_SOCKETS_GET_LOCAL_CID, request; want != got {
			t.Fatalf("unexpected request number for ioctl:\n- want: %x\n-  got: %x",
				want, got)
		}

		cidp, ok := cfds[ioctlFD]
		if !ok {
			t.Fatal("ioctl file descriptor not found in map")
		}

		*cidp = contextID
		return nil
	}

	fs := &testFS{
		open: func(name string) (*os.File, error) {
			return os.NewFile(fd, name), nil
		},
		ioctl: ioctl,
	}

	if err := sysContextID(fs, &cid); err != nil {
		t.Fatalf("failed to retrieve host's context ID: %v", err)
	}

	if want, got := contextID, cid; want != got {
		t.Fatalf("unexpected context ID:\n- want: %d\n-  got: %d",
			want, got)
	}
}

var _ fs = &testFS{}

// A testFS is the testing implementation of fs.
type testFS struct {
	open  func(name string) (*os.File, error)
	ioctl func(fd uintptr, request int, argp unsafe.Pointer) error
}

func (fs *testFS) Open(name string) (*os.File, error) {
	return fs.open(name)
}

func (fs *testFS) Ioctl(fd uintptr, request int, argp unsafe.Pointer) error {
	return fs.ioctl(fd, request, argp)
}

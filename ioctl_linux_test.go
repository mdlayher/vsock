// +build linux

package vsock

import (
	"os"
	"testing"
)

func Test_localContextIDGuest(t *testing.T) {
	const (
		fd        uintptr = 10
		contextID uint32  = 5
	)

	// Since it isn't safe to manipulate the argument pointer with
	// ioctl, we just check that the ioctl performs its commands
	// on the appropriate file descriptor and request number.
	//
	// TODO(mdlayher): An option would be to pass a *uint32
	// to localContextID to enable testing this with a map[uintptr]*uint32.
	ioctl := func(ioctlFD uintptr, request int, _ uintptr) error {
		if want, got := fd, ioctlFD; want != got {
			t.Fatalf("unexpected file descriptor for ioctl:\n- want: %d\n-  got: %d",
				want, got)
		}

		if want, got := ioctlGetLocalCID, request; want != got {
			t.Fatalf("unexpected request number for ioctl:\n- want: %x\n-  got: %x",
				want, got)
		}

		return nil
	}

	_, err := localContextID(&testFS{
		open: func(name string) (*os.File, error) {
			return os.NewFile(fd, name), nil
		},
		ioctl: ioctl,
	})
	if err != nil {
		t.Fatalf("failed to retrieve host's context ID: %v", err)
	}
}

func Test_localContextIDGuestIntegration(t *testing.T) {
	if !devVsockExists(t) {
		t.Skipf("machine does not have %q, skipping guest integration test", devVsock)
	}

	cid, err := localContextID(sysFS{})
	if err != nil {
		t.Fatalf("failed to retrieve guest's context ID: %v", err)
	}

	// Guests should always have a context ID of 3 or more, since
	// 0-2 are invalid or reserved.
	if cid < 3 {
		t.Fatalf("unexpected guest context ID: %d", cid)
	}
}

func Test_localContextIDHost(t *testing.T) {
	cid, err := localContextID(&testFS{
		// Pretend that device doesn't exist, like on a hypervisor
		//
		// TODO(mdlayher): also differentiate between a hypervisor versus a machine
		// without the kernel modules installed.
		open: func(_ string) (*os.File, error) {
			return nil, os.ErrNotExist
		},
	})
	if err != nil {
		t.Fatalf("failed to retrieve host's context ID: %v", err)
	}

	if want, got := ContextIDHost, cid; want != got {
		t.Fatalf("unexpected host context ID:\n- want: %d\n-  got: %d",
			want, got)
	}
}

func Test_localContextIDHostIntegration(t *testing.T) {
	if devVsockExists(t) {
		t.Skipf("machine has %q, skipping host integration test", devVsock)
	}

	cid, err := localContextID(sysFS{})
	if err != nil {
		t.Fatalf("failed to retrieve host's context ID: %v", err)
	}

	if want, got := ContextIDHost, cid; want != got {
		t.Fatalf("unexpected host context ID:\n- want: %d\n-  got: %d",
			want, got)
	}
}

func devVsockExists(t *testing.T) bool {
	_, err := os.Stat(devVsock)
	switch {
	case os.IsNotExist(err):
		return false
	case err != nil:
		t.Fatalf("failed to check for %q: %v", devVsock, err)
	}

	return true
}

var _ fs = &testFS{}

// A testFS is the testing implementation of fs.
type testFS struct {
	open  func(name string) (*os.File, error)
	ioctl func(fd uintptr, request int, argp uintptr) error
}

func (fs *testFS) Open(name string) (*os.File, error) {
	return fs.open(name)
}

func (fs *testFS) Ioctl(fd uintptr, request int, argp uintptr) error {
	return fs.ioctl(fd, request, argp)
}

// Package vsutil provides added functionality for package vsock-internal use.
package vsutil

import (
	"net"
	"os"
	"testing"

	"github.com/mdlayher/vsock"
)

// IsHypervisor detects if this machine is a hypervisor by determining if
// /dev/vsock is available, and then if its context ID matches the one assigned
// to hosts.
func IsHypervisor(t *testing.T) bool {
	t.Helper()

	cid, err := vsock.ContextID()
	if err != nil {
		SkipDeviceError(t, err)

		t.Fatalf("failed to retrieve context ID: %v", err)
	}

	return cid == vsock.Host
}

// SkipDeviceError skips this test if err is related to a failure to access the
// /dev/vsock device.
func SkipDeviceError(t *testing.T, err error) {
	t.Helper()

	// Unwrap net.OpError if needed.
	// TODO(mdlayher): errors.Unwrap in Go 1.13.
	if nerr, ok := err.(*net.OpError); ok {
		err = nerr.Err
	}

	if os.IsNotExist(err) {
		t.Skipf("skipping, vsock device does not exist (try: 'modprobe vhost_vsock'): %v", err)
	}
	if os.IsPermission(err) {
		t.Skipf("skipping, permission denied (try: 'chmod 666 /dev/vsock'): %v", err)
	}
}

// SkipHostIntegration skips this test if this machine is a host and cannot
// perform a given test.
func SkipHostIntegration(t *testing.T) {
	t.Helper()

	if IsHypervisor(t) {
		t.Skip("skipping, this integration test must be run in a guest")
	}
}

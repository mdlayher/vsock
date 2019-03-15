package vsock

import (
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestAddr_fileName(t *testing.T) {
	tests := []struct {
		cid  uint32
		port uint32
		s    string
	}{
		{
			cid:  ContextIDHypervisor,
			port: 10,
			s:    "vsock:hypervisor(0):10",
		},
		{
			cid:  ContextIDReserved,
			port: 20,
			s:    "vsock:reserved(1):20",
		},
		{
			cid:  ContextIDHost,
			port: 30,
			s:    "vsock:host(2):30",
		},
		{
			cid:  3,
			port: 40,
			s:    "vsock:vm(3):40",
		},
	}

	for _, tt := range tests {
		t.Run(tt.s, func(t *testing.T) {
			addr := &Addr{
				ContextID: tt.cid,
				Port:      tt.port,
			}

			if want, got := tt.s, addr.fileName(); want != got {
				t.Fatalf("unexpected file name:\n- want: %q\n-  got: %q",
					want, got)
			}
		})
	}
}

func TestUnblockAcceptAfterClose(t *testing.T) {
	listener, err := Listen(1024)
	if err != nil {
		if os.IsNotExist(err) {
			t.Skipf("skipping, vsock device does not exist: %v", err)
		}
		if os.IsPermission(err) {
			t.Skipf("skipping, permission denied: %v", err)
		}

		t.Fatalf("failed to run listener: %v", err)
	}

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()

		t.Log("start accept")
		_, err := listener.Accept()
		t.Log("after accept")

		if !strings.Contains(err.Error(), "use of closed file") {
			t.Errorf("unexpected accept error: %v", err)
		}
	}()

	time.AfterFunc(10*time.Second, func() {
		panic("took too long waiting for listener to close")
	})

	time.Sleep(100 * time.Millisecond)

	if err := listener.Close(); err != nil {
		t.Fatalf("failed to close listener: %v", err)
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		t.Log("done")
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting accept to unblock")
	}
}

package vsock

import (
	"sync"
	"testing"
	"time"

	"golang.org/x/sys/unix"
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
		t.Fatalf("failed to run listener: %v", err)
	}

	var wg sync.WaitGroup

	wg.Add(1)

	go func() {
		defer wg.Done()

		t.Log("start accept")
		_, err := listener.Accept()
		t.Log("after accept")

		if err == nil {
			t.Error("accept should return an error, got nil")
		} else if err != unix.EWOULDBLOCK {
			t.Errorf("unexpected error '%v' != '%v'", err, unix.EWOULDBLOCK)
		}
	}()

	time.Sleep(100 * time.Millisecond)

	if err := listener.Close(); err != nil {
		t.Fatalf("failed to close listener: %v", err)
	}

	done := make(chan bool)
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		t.Log("done")
		return
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting accept to unblock")
	}
}

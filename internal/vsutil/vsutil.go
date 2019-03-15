// Package vsutil provides added functionality for package vsock-internal use.
package vsutil

import (
	"net"
	"time"
)

// Accept blocks until a single connection is accepted by the net.Listener, and
// then closes the net.Listener.  If timeout is non-zero, the listener will be
// closed after the timeout expires, even if no connection was accepted.
func Accept(l net.Listener, timeout time.Duration) (net.Conn, error) {
	defer l.Close()

	// This function accomodates both Go1.12+ and Go1.11- functionality to allow
	// net.Listener.Accept to be canceled by net.Listener.Close.
	//
	// If a timeout is set, set up a timer to close the listener and either:
	// - Go1.12+: unblock the call to Accept
	// - Go1.11 : eventually halt the loop due to closed file descriptor
	cancel := func() {}
	if timeout != 0 {
		timer := time.AfterFunc(timeout, func() { _ = l.Close() })
		cancel = func() { timer.Stop() }
	}

	for {
		c, err := l.Accept()
		if err != nil {
			if nerr, ok := err.(net.Error); ok && nerr.Temporary() {
				time.Sleep(250 * time.Millisecond)
				continue
			}

			return nil, err
		}

		// Got a connection, stop the timer.
		cancel()
		return c, nil
	}
}

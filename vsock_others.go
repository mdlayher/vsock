//+build !linux

package vsock

import (
	"fmt"
	"net"
	"runtime"
	"time"
)

var (
	// errUnimplemented is returned by all functions on platforms that
	// cannot make use of VM sockets.
	errUnimplemented = fmt.Errorf("vsock: not implemented on %s/%s",
		runtime.GOOS, runtime.GOARCH)
)

func listenStream(_ uint32) (*Listener, error) { return nil, errUnimplemented }

type listener struct{}

func (*listener) Accept() (net.Conn, error)     { return nil, errUnimplemented }
func (*listener) Addr() net.Addr                { return nil }
func (*listener) Close() error                  { return errUnimplemented }
func (*listener) SetDeadline(_ time.Time) error { return errUnimplemented }

func dialStream(_, _ uint32) (*Conn, error) { return nil, errUnimplemented }

type conn struct{}

func (*conn) LocalAddr() net.Addr                { return nil }
func (*conn) RemoteAddr() net.Addr               { return nil }
func (*conn) SetDeadline(_ time.Time) error      { return errUnimplemented }
func (*conn) SetReadDeadline(_ time.Time) error  { return errUnimplemented }
func (*conn) SetWriteDeadline(_ time.Time) error { return errUnimplemented }
func (*conn) Read(_ []byte) (int, error)         { return 0, errUnimplemented }
func (*conn) Write(_ []byte) (int, error)        { return 0, errUnimplemented }
func (*conn) Close() error                       { return errUnimplemented }
func (*conn) CloseRead() error                   { return errUnimplemented }
func (*conn) CloseWrite() error                  { return errUnimplemented }

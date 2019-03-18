//+build linux

package vsock

import (
	"time"

	"golang.org/x/sys/unix"
)

var _ listenFD = &testListenFD{}

type testListenFD struct {
	accept4        func(flags int) (connFD, unix.Sockaddr, error)
	bind           func(sa unix.Sockaddr) error
	close          func() error
	listen         func(n int) error
	getsockname    func() (unix.Sockaddr, error)
	setNonblocking func(name string) error
	setDeadline    func(t time.Time) error
}

func (lfd *testListenFD) Accept4(flags int) (connFD, unix.Sockaddr, error) { return lfd.accept4(flags) }
func (lfd *testListenFD) Bind(sa unix.Sockaddr) error                      { return lfd.bind(sa) }
func (lfd *testListenFD) Close() error                                     { return lfd.close() }
func (lfd *testListenFD) EarlyClose() error {
	// Share logic with close.
	return lfd.close()
}
func (lfd *testListenFD) Getsockname() (unix.Sockaddr, error) { return lfd.getsockname() }
func (lfd *testListenFD) Listen(n int) error                  { return lfd.listen(n) }
func (lfd *testListenFD) SetNonblocking(name string) error    { return lfd.setNonblocking(name) }
func (lfd *testListenFD) SetDeadline(t time.Time) error       { return lfd.setDeadline(t) }

var _ connFD = &testConnFD{}

type testConnFD struct {
	read           func(b []byte) (int, error)
	write          func(b []byte) (int, error)
	close          func() error
	connect        func(sa unix.Sockaddr) error
	getsockname    func() (unix.Sockaddr, error)
	setNonblocking func(name string) error
	setDeadline    func(t time.Time, typ deadlineType) error
	shutdown       func(how int) error
}

func (cfd *testConnFD) Read(b []byte) (int, error)  { return cfd.read(b) }
func (cfd *testConnFD) Write(b []byte) (int, error) { return cfd.write(b) }
func (cfd *testConnFD) Close() error                { return cfd.close() }
func (cfd *testConnFD) EarlyClose() error {
	// Share logic with close.
	return cfd.close()
}
func (cfd *testConnFD) Connect(sa unix.Sockaddr) error      { return cfd.connect(sa) }
func (cfd *testConnFD) Getsockname() (unix.Sockaddr, error) { return cfd.getsockname() }
func (cfd *testConnFD) SetNonblocking(name string) error    { return cfd.setNonblocking(name) }
func (cfd *testConnFD) SetDeadline(t time.Time, typ deadlineType) error {
	return cfd.setDeadline(t, typ)
}
func (cfd *testConnFD) Shutdown(how int) error { return cfd.shutdown(how) }

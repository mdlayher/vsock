package vsock

import (
	"errors"
	"fmt"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	// afVsock is the address family used by the viosock driver.
	afVsock = 40

	// viosockName is the device exposed by the viosock driver.
	viosockName = `\\.\Viosock`

	// vmaddrPortAny lets the driver pick an available port.
	vmaddrPortAny = 0xffffffff

	// ioctlGetConfig retrieves the configuration of the viosock device.
	ioctlGetConfig = 0x08013004

	// socketError is returned by Winsock2 functions on failure.
	socketError = -1

	// maxRW is the largest buffer passed to a single recv or send call.
	maxRW = 1 << 30
)

// Winsock2 network events.
const (
	fdRead   = 0x01
	fdWrite  = 0x02
	fdAccept = 0x08
	fdClose  = 0x20
)

// I/O directions.
const (
	modeRead = iota
	modeWrite
)

// modeEvents are the network events relevant to each I/O direction.
var modeEvents = [2]int32{
	modeRead:  fdRead | fdAccept | fdClose,
	modeWrite: fdWrite | fdClose,
}

// errUnimplemented is returned by functions which are not supported on Windows.
var errUnimplemented = errors.New("vsock: not implemented on windows")

var (
	modws2_32               = windows.NewLazySystemDLL("ws2_32.dll")
	procSocket              = modws2_32.NewProc("socket")
	procBind                = modws2_32.NewProc("bind")
	procAccept              = modws2_32.NewProc("accept")
	procConnect             = modws2_32.NewProc("connect")
	procSend                = modws2_32.NewProc("send")
	procRecv                = modws2_32.NewProc("recv")
	procGetsockname         = modws2_32.NewProc("getsockname")
	procWSAEventSelect      = modws2_32.NewProc("WSAEventSelect")
	procWSAEnumNetworkEvent = modws2_32.NewProc("WSAEnumNetworkEvents")
)

// sockaddrVM is the sockaddr_vm structure of the viosock driver.
type sockaddrVM struct {
	family uint16
	_      uint16
	port   uint32
	cid    uint32
}

// errnoErr converts the error number of a failed call into an error.
func errnoErr(e syscall.Errno) error {
	switch e {
	case 0:
		return syscall.EINVAL
	case windows.WSAEWOULDBLOCK:
		return syscall.EAGAIN
	}

	return e
}

// failed reports whether a Winsock2 function returning an int failed.
func failed(r1 uintptr) bool {
	return int32(r1) == socketError
}

func socket(af, typ, protocol int32) (windows.Handle, error) {
	r1, _, e1 := syscall.SyscallN(procSocket.Addr(), uintptr(af), uintptr(typ), uintptr(protocol))
	if windows.Handle(r1) == windows.InvalidHandle {
		return windows.InvalidHandle, errnoErr(e1)
	}

	return windows.Handle(r1), nil
}

func bind(fd windows.Handle, sa *sockaddrVM) error {
	r1, _, e1 := syscall.SyscallN(procBind.Addr(), uintptr(fd), uintptr(unsafe.Pointer(sa)), unsafe.Sizeof(*sa))
	if failed(r1) {
		return errnoErr(e1)
	}

	return nil
}

func connect(fd windows.Handle, sa *sockaddrVM) error {
	r1, _, e1 := syscall.SyscallN(procConnect.Addr(), uintptr(fd), uintptr(unsafe.Pointer(sa)), unsafe.Sizeof(*sa))
	if failed(r1) {
		return errnoErr(e1)
	}

	return nil
}

func accept(fd windows.Handle) (windows.Handle, *sockaddrVM, error) {
	sa := &sockaddrVM{}
	salen := int32(unsafe.Sizeof(*sa))

	r1, _, e1 := syscall.SyscallN(procAccept.Addr(), uintptr(fd), uintptr(unsafe.Pointer(sa)), uintptr(unsafe.Pointer(&salen)))
	if windows.Handle(r1) == windows.InvalidHandle {
		return windows.InvalidHandle, nil, errnoErr(e1)
	}

	return windows.Handle(r1), sa, nil
}

func getsockname(fd windows.Handle) (*Addr, error) {
	sa := &sockaddrVM{}
	salen := int32(unsafe.Sizeof(*sa))

	r1, _, e1 := syscall.SyscallN(procGetsockname.Addr(), uintptr(fd), uintptr(unsafe.Pointer(sa)), uintptr(unsafe.Pointer(&salen)))
	if failed(r1) {
		return nil, errnoErr(e1)
	}

	return &Addr{ContextID: sa.cid, Port: sa.port}, nil
}

func recv(fd windows.Handle, b []byte) (int, error) {
	if len(b) == 0 {
		return 0, nil
	}

	r1, _, e1 := syscall.SyscallN(procRecv.Addr(), uintptr(fd), uintptr(unsafe.Pointer(&b[0])), uintptr(min(len(b), maxRW)), 0)
	if failed(r1) {
		return 0, errnoErr(e1)
	}

	return int(int32(r1)), nil
}

func send(fd windows.Handle, b []byte) (int, error) {
	if len(b) == 0 {
		return 0, nil
	}

	r1, _, e1 := syscall.SyscallN(procSend.Addr(), uintptr(fd), uintptr(unsafe.Pointer(&b[0])), uintptr(min(len(b), maxRW)), 0)
	if failed(r1) {
		return 0, errnoErr(e1)
	}

	return int(int32(r1)), nil
}

// eventSelect requests that an event be signaled on network events. This
// also makes the socket non-blocking.
func eventSelect(fd, event windows.Handle, events int32) error {
	r1, _, e1 := syscall.SyscallN(procWSAEventSelect.Addr(), uintptr(fd), uintptr(event), uintptr(events))
	if failed(r1) {
		return errnoErr(e1)
	}

	return nil
}

// networkEvents is the Winsock2 WSANETWORKEVENTS structure.
type networkEvents struct {
	events int32
	errors [10]int32
}

// enumNetworkEvents returns the pending network events and resets the event.
func enumNetworkEvents(fd, event windows.Handle) (int32, error) {
	var ne networkEvents

	r1, _, e1 := syscall.SyscallN(procWSAEnumNetworkEvent.Addr(), uintptr(fd), uintptr(event), uintptr(unsafe.Pointer(&ne)))
	if failed(r1) {
		return 0, errnoErr(e1)
	}

	return ne.events, nil
}

// A netFD is a non-blocking socket which can wait for network events.
//
// Base Winsock2 I/O can't be canceled, so pending I/O instead waits on
// events which are also signaled when closing or changing a deadline.
type netFD struct {
	// mu is held for reading during I/O and for writing to close the handles.
	mu     sync.RWMutex
	closed atomic.Bool

	fd    windows.Handle
	event windows.Handle

	// Per I/O direction state.
	wakes     [2]windows.Handle
	deadlines [2]deadline
}

// newFD takes ownership of a socket, closing it on failure.
func newFD(fd windows.Handle) (*netFD, error) {
	f := &netFD{fd: fd}

	err := f.init()
	if err != nil {
		_ = f.closeHandles()
		return nil, err
	}

	return f, nil
}

func (f *netFD) init() error {
	var err error

	// The network event is reset when retrieving the events.
	f.event, err = windows.CreateEvent(nil, 1, 0, nil)
	if err != nil {
		return os.NewSyscallError("CreateEvent", err)
	}

	for i := range f.wakes {
		f.wakes[i], err = windows.CreateEvent(nil, 0, 0, nil)
		if err != nil {
			return os.NewSyscallError("CreateEvent", err)
		}
	}

	err = eventSelect(f.fd, f.event, fdRead|fdWrite|fdAccept|fdClose)
	if err != nil {
		return os.NewSyscallError("WSAEventSelect", err)
	}

	return nil
}

func (f *netFD) closeHandles() error {
	err := windows.Closesocket(f.fd)

	for _, h := range append(f.wakes[:], f.event) {
		if h != 0 {
			_ = windows.CloseHandle(h)
		}
	}

	return err
}

// close interrupts any pending I/O and closes the socket.
func (f *netFD) close() error {
	if !f.closed.CompareAndSwap(false, true) {
		return net.ErrClosed
	}

	for _, h := range f.wakes {
		_ = windows.SetEvent(h)
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	return f.closeHandles()
}

// lock prevents the socket from being closed until unlock is called.
func (f *netFD) lock() error {
	f.mu.RLock()

	if f.closed.Load() {
		f.mu.RUnlock()
		return net.ErrClosed
	}

	return nil
}

func (f *netFD) unlock() {
	f.mu.RUnlock()
}

// setDeadline changes a deadline and has any pending I/O notice it.
func (f *netFD) setDeadline(mode int, t time.Time) error {
	err := f.lock()
	if err != nil {
		return err
	}
	defer f.unlock()

	f.deadlines[mode].set(t)

	return windows.SetEvent(f.wakes[mode])
}

// check reports whether I/O can be attempted.
func (f *netFD) check(mode int) error {
	if f.closed.Load() {
		return net.ErrClosed
	}

	t := f.deadlines[mode].get()
	if !t.IsZero() && !time.Now().Before(t) {
		return os.ErrDeadlineExceeded
	}

	return nil
}

// wait blocks until I/O should be attempted again, which the caller must
// precede with a call to check.
func (f *netFD) wait(mode int) error {
	timeout := uint32(windows.INFINITE)

	t := f.deadlines[mode].get()
	if !t.IsZero() {
		// Round up to not wake up before the deadline.
		ms := (time.Until(t) + time.Millisecond - 1) / time.Millisecond
		timeout = uint32(max(min(ms, windows.INFINITE-1), 0))
	}

	n, err := windows.WaitForMultipleObjects([]windows.Handle{f.event, f.wakes[mode]}, false, timeout)
	if err != nil {
		return os.NewSyscallError("WaitForMultipleObjects", err)
	}

	if n != windows.WAIT_OBJECT_0 {
		return nil
	}

	events, err := enumNetworkEvents(f.fd, f.event)
	if err != nil {
		return os.NewSyscallError("WSAEnumNetworkEvents", err)
	}

	// The events are only reported once, so pass them on to the other direction.
	other := 1 - mode
	if events&modeEvents[other] != 0 {
		err = windows.SetEvent(f.wakes[other])
		if err != nil {
			return os.NewSyscallError("SetEvent", err)
		}
	}

	return nil
}

// A deadline is a point in time which can be safely changed during I/O.
type deadline struct {
	mu sync.Mutex
	t  time.Time
}

func (d *deadline) set(t time.Time) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.t = t
}

func (d *deadline) get() time.Time {
	d.mu.Lock()
	defer d.mu.Unlock()

	return d.t
}

// isErrno determines if an error matches a UNIX error number.
func isErrno(err error, errno int) bool {
	switch errno {
	case ebadf:
		return err == windows.WSAENOTSOCK
	case enotconn:
		return err == windows.WSAENOTCONN
	default:
		return false
	}
}

// contextID retrieves the local context ID for this system.
func contextID() (uint32, error) {
	name, err := windows.UTF16PtrFromString(viosockName)
	if err != nil {
		return 0, err
	}

	dev, err := windows.CreateFile(name, windows.GENERIC_READ, windows.FILE_SHARE_READ, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return 0, &os.PathError{Op: "open", Path: viosockName, Err: err}
	}

	defer func() { _ = windows.CloseHandle(dev) }()

	// VIRTIO_VSOCK_CONFIG structure of the viosock driver.
	var config struct {
		guestCID uint32
	}

	var n uint32

	err = windows.DeviceIoControl(dev, ioctlGetConfig, nil, 0, (*byte)(unsafe.Pointer(&config)), uint32(unsafe.Sizeof(config)), &n, nil)
	if err != nil {
		return 0, fmt.Errorf("get %s config: %w", viosockName, err)
	}

	return config.guestCID, nil
}

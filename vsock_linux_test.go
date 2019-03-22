package vsock

import (
	"errors"
	"io"
	"net"
	"os"
	"testing"

	"github.com/google/go-cmp/cmp"
	"golang.org/x/sys/unix"
)

func Test_opError(t *testing.T) {
	// The default op for empty op fields.
	const defaultOp = "read"

	var (
		// Unfortunate, but string matching it is for now.
		errClosed = errors.New("use of closed network connection")

		local = &Addr{
			ContextID: Host,
			Port:      1024,
		}

		remote = &Addr{
			ContextID: 3,
			Port:      2048,
		}
	)

	tests := []struct {
		name   string
		op     string
		err    error
		local  net.Addr
		remote net.Addr
		want   error
	}{
		{
			name: "nil error",
		},
		{
			name: "unknown",
			err:  errors.New("foo"),
			want: &net.OpError{
				Err: errors.New("foo"),
			},
		},
		{
			name: "EOF",
			err:  io.EOF,
			want: io.EOF,
		},
		{
			name: "ENOTCONN",
			err:  unix.ENOTCONN,
			want: io.EOF,
		},
		{
			name: "PathError ENOTCONN",
			err: &os.PathError{
				Err: unix.ENOTCONN,
			},
			want: io.EOF,
		},
		{
			name: "ErrClosed",
			err:  os.ErrClosed,
			want: &net.OpError{
				Err: errClosed,
			},
		},
		{
			name: "EBADF",
			err:  unix.EBADF,
			want: &net.OpError{
				Err: errClosed,
			},
		},
		{
			name: "string use of closed",
			err:  errors.New("use of closed file"),
			want: &net.OpError{
				Err: errClosed,
			},
		},
		{
			name: "special PathError /dev/vsock",
			err: &os.PathError{
				Op:   "open",
				Path: devVsock,
				Err:  unix.ENOENT,
			},
			want: &net.OpError{
				Err: &os.PathError{
					Op:   "open",
					Path: devVsock,
					Err:  unix.ENOENT,
				},
			},
		},
		{
			name:   "op close",
			op:     opClose,
			err:    errClosed,
			local:  local,
			remote: remote,
			want: &net.OpError{
				Op:     opClose,
				Source: local,
				Addr:   remote,
				Err:    errClosed,
			},
		},
		{
			name:   "op dial",
			op:     opDial,
			err:    errClosed,
			local:  local,
			remote: remote,
			want: &net.OpError{
				Op:     opDial,
				Source: local,
				Addr:   remote,
				Err:    errClosed,
			},
		},
		{
			name:   "op read",
			op:     opRead,
			err:    errClosed,
			local:  local,
			remote: remote,
			want: &net.OpError{
				Op:     opRead,
				Source: local,
				Addr:   remote,
				Err:    errClosed,
			},
		},
		{
			name:   "op write",
			op:     opWrite,
			err:    errClosed,
			local:  local,
			remote: remote,
			want: &net.OpError{
				Op:     opWrite,
				Source: local,
				Addr:   remote,
				Err:    errClosed,
			},
		},
		{
			name:  "op accept",
			op:    opAccept,
			err:   errClosed,
			local: local,
			want: &net.OpError{
				Op:   opAccept,
				Addr: local,
				Err:  errClosed,
			},
		},
		{
			name:  "op listen",
			op:    opListen,
			err:   errClosed,
			local: local,
			want: &net.OpError{
				Op:   opListen,
				Addr: local,
				Err:  errClosed,
			},
		},
		{
			name:  "op set",
			op:    opSet,
			err:   errClosed,
			local: local,
			want: &net.OpError{
				Op:   opSet,
				Addr: local,
				Err:  errClosed,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			op := tt.op
			if op == "" {
				op = defaultOp
			}

			err := opError(op, tt.err, tt.local, tt.remote)
			if err == nil {
				if tt.want != nil {
					t.Fatal("expected an output error, but none occurred")
				}

				return
			}

			// Populate sane defaults to save some typing.
			want := tt.want
			if nerr, ok := tt.want.(*net.OpError); ok {
				if nerr.Op == "" {
					nerr.Op = defaultOp
				}

				if nerr.Net == "" {
					nerr.Net = network
				}

				want = nerr
			}

			if diff := cmp.Diff(want, err, cmp.Comparer(errorsEqual)); diff != "" {
				t.Fatalf("unexpected error (-want +got):\n%s", diff)
			}
		})
	}
}

func errorsEqual(x, y error) bool {
	if x == nil || y == nil {
		return x == nil && y == nil
	}

	return x.Error() == y.Error()
}

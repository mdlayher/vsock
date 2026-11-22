//go:build !linux && !windows
// +build !linux,!windows

package vsock

import (
	"context"
	"testing"
)

func TestUnimplemented(t *testing.T) {
	want := errUnimplemented

	if _, got := ContextID(); want != got {
		t.Fatalf("unexpected error from ContextID:\n- want: %v\n-  got: %v",
			want, got)
	}

	if _, got := listen(0, 0, nil); want != got {
		t.Fatalf("unexpected error from listen:\n- want: %v\n-  got: %v",
			want, got)
	}

	if _, got := dial(context.Background(), 0, 0, nil); want != got {
		t.Fatalf("unexpected error from dial:\n- want: %v\n-  got: %v",
			want, got)
	}
}

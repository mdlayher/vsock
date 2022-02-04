//go:build !linux
// +build !linux

package vsock

import "testing"

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

	if _, got := dial(0, 0, nil); want != got {
		t.Fatalf("unexpected error from dial:\n- want: %v\n-  got: %v",
			want, got)
	}
}

package mapping

import (
	"fmt"
	"io"
	"os"
	"syscall"
	"testing"
)

// // // // // // // // // //

func TestIsErrorAddressAlreadyInUse(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
		{
			name: "io.EOF",
			err:  io.EOF,
			want: false,
		},
		{
			name: "random error",
			err:  fmt.Errorf("something failed"),
			want: false,
		},
		{
			name: "EADDRINUSE",
			err:  &os.SyscallError{Syscall: "bind", Err: syscall.EADDRINUSE},
			want: true,
		},
		{
			name: "ECONNREFUSED",
			err:  &os.SyscallError{Syscall: "connect", Err: syscall.ECONNREFUSED},
			want: false,
		},
		{
			name: "wrapped EADDRINUSE",
			err:  fmt.Errorf("listen: %w", &os.SyscallError{Syscall: "bind", Err: syscall.EADDRINUSE}),
			want: true,
		},
		{
			name: "wrapped non-EADDRINUSE",
			err:  fmt.Errorf("listen: %w", &os.SyscallError{Syscall: "bind", Err: syscall.EPERM}),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isErrorAddressAlreadyInUse(tt.err)
			if got != tt.want {
				t.Errorf("isErrorAddressAlreadyInUse(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

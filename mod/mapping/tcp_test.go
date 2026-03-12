package mapping

import (
	"fmt"
	"io"
	"testing"
)

// // // // // // // // // //

// transientNetErrObj simulates a transient network error.
type transientNetErrObj struct{ msg string }

func (e *transientNetErrObj) Error() string   { return e.msg }
func (e *transientNetErrObj) Timeout() bool   { return false }
func (e *transientNetErrObj) Temporary() bool { return true } //nolint:staticcheck

// permanentNetErrObj simulates a permanent network error.
type permanentNetErrObj struct{ msg string }

func (e *permanentNetErrObj) Error() string   { return e.msg }
func (e *permanentNetErrObj) Timeout() bool   { return false }
func (e *permanentNetErrObj) Temporary() bool { return false } //nolint:staticcheck

// //

func TestIsTransientAcceptError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "nil",
			err:  nil,
			want: false,
		},
		{
			name: "non-net error",
			err:  io.EOF,
			want: false,
		},
		{
			name: "plain error",
			err:  fmt.Errorf("some error"),
			want: false,
		},
		{
			name: "transient net.Error",
			err:  &transientNetErrObj{msg: "ECONNABORTED"},
			want: true,
		},
		{
			name: "permanent net.Error",
			err:  &permanentNetErrObj{msg: "EMFILE"},
			want: false,
		},
		{
			name: "wrapped transient net.Error",
			err:  fmt.Errorf("accept: %w", &transientNetErrObj{msg: "ECONNABORTED"}),
			want: true,
		},
		{
			name: "wrapped permanent net.Error",
			err:  fmt.Errorf("accept: %w", &permanentNetErrObj{msg: "EMFILE"}),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isTransientAcceptError(tt.err)
			if got != tt.want {
				t.Errorf("isTransientAcceptError(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

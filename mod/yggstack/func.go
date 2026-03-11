package yggstack

import (
	"errors"
	"os"
	"runtime"
	"syscall"
)

// // // // // // // // // //

func isErrorAddressAlreadyInUse(err error) bool {
	var eOsSyscall *os.SyscallError
	if !errors.As(err, &eOsSyscall) {
		return false
	}
	var errErrno syscall.Errno
	if !errors.As(eOsSyscall, &errErrno) {
		return false
	}
	if errors.Is(errErrno, syscall.EADDRINUSE) {
		return true
	}
	const WSAEADDRINUSE = 10048
	if runtime.GOOS == "windows" && errErrno == WSAEADDRINUSE {
		return true
	}
	return false
}

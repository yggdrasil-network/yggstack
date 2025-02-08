//go:build cgo
// +build cgo

package main

/*
#include <stdlib.h>
*/
import "C"
import (
	"unsafe"

	"github.com/yggdrasil-network/yggstack/src/driver"
)

func main() {}

//export YggstackMain
func YggstackMain(argc C.int, argv **C.char) {
	length := int(argc)
	args := make([]string, length)
	argvPtr := unsafe.Pointer(argv)
	for i := 0; i < length; i++ {
		arg := *(**C.char)(unsafe.Add(argvPtr, uintptr(i)*unsafe.Sizeof((*C.char)(nil))))
		args[i] = C.GoString(arg)
	}
	driver.Run(args)
}

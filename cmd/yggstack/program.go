package main

import (
	"os"

	"github.com/yggdrasil-network/yggstack/src/driver"
)

func main() {
	driver.Run(os.Args[1:])
}

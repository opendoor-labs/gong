// +build ignore

package main

import (
	"flag"
	"fmt"

	"github.com/opendoor-labs/gong/Godeps/_workspace/src/github.com/kidoman/embd"

	_ "github.com/opendoor-labs/gong/Godeps/_workspace/src/github.com/kidoman/embd/host/all"
)

func main() {
	flag.Parse()

	embd.InitGPIO()
	defer embd.CloseGPIO()

	val, _ := embd.AnalogRead(0)
	fmt.Printf("Reading: %v\n", val)
}

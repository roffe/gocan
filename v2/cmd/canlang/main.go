// Command canlang runs a CANLang (Lua) script against a CAN bus.
//
//	canlang -list
//	canlang -adapter "SocketCAN vcan0" script.lua
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"

	gocan "github.com/roffe/gocan/v2"
	_ "github.com/roffe/gocan/v2/adapters/all"
	_ "github.com/roffe/gocan/v2/adapters/combi"
	"github.com/roffe/gocan/v2/canlang"
)

func main() {
	adapter := flag.String("adapter", "loopback", "adapter name (see -list)")
	port := flag.String("port", "", "port name, if the adapter needs one")
	rate := flag.Float64("rate", 500, "CAN bus rate in kbit/s")
	list := flag.Bool("list", false, "list available adapters and exit")
	flag.Parse()

	if *list {
		for _, name := range gocan.AdapterNames() {
			fmt.Println(name)
		}
		return
	}
	if flag.NArg() < 1 {
		log.Fatal("usage: canlang [flags] script.lua [args...]")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	bus, err := gocan.Open(ctx, *adapter, gocan.Config{Port: *port, CANRate: *rate})
	if err != nil {
		log.Fatal(err)
	}
	defer bus.Close()

	if err := canlang.Run(ctx, bus, flag.Arg(0), flag.Args()[1:]...); err != nil {
		log.Fatal(err)
	}
}

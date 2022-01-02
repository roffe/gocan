package main

import (
	"log"
	"os"
	"os/signal"

	"github.com/roffe/canusb"
	"github.com/roffe/canusb/pkg/t7"
	flag "github.com/spf13/pflag"
)

var (
	port     string
	baudrate int
)

func init() {
	log.SetFlags(log.Lshortfile | log.LstdFlags)
	flag.StringVarP(&port, "port", "p", "", "COM-port, Windows COM#\nLinux/OSX: /dev/ttyUSB#")
	flag.IntVarP(&baudrate, "baudrate", "b", 115200, "Baudrate")
	flag.Parse()
}

func main() {
	// Setup interupt handler for ctrl-c
	quitChan := make(chan os.Signal, 1)
	signal.Notify(quitChan, os.Interrupt)
	// Create new canbus client
	c, err := canusb.New(
		// Set com-port options
		canusb.OptComPort(port, baudrate),
		// Set CAN bit-rate
		canusb.OptRate(t7.PBusRate),
	)
	if err != nil {
		log.Fatal(err)
	}
	defer c.Stop()
	if err := t7.TrionicDataInitialization(c); err != nil {
		log.Fatal(err)
	}
	t7.Dumperino(c)
	run(c, quitChan)
	st := c.Stats()
	log.Println(st.String())
}

func run(c *canusb.Canusb, quitChan chan os.Signal) {
outer:
	for {
		select {
		case f := <-c.Read():
			switch f.Identifier {
			case 0x1A0:
				log.Println(f.String())
			case 0x238: // Trionic data initialization reply
				log.Println(f.String())
			case 0x258: // Trionic data query reply
				log.Println(f.String())
			default:
				log.Println(f.String())
			}
		case s := <-quitChan:
			log.Printf("got %v, stopping CAN communication", s)
			break outer
		}
	}
}

package main

import (
	"log"
	"os"
	"os/signal"
	"time"

	"github.com/roffe/canusb"
	flag "github.com/spf13/pflag"
)

var (
	port     string
	baudrate int
)

func init() {
	log.SetFlags(log.Lshortfile | log.LstdFlags)
	flag.StringVarP(&port, "port", "p", "COM1", "Comport, Windows COM#.. Linux/OSX: /dev/ttyUS#..")
	flag.IntVarP(&baudrate, "baudrate", "b", 115200, "Baudrate")
	flag.Parse()
}

func main() {
	defer log.Println("exited")
	// Setup interupt handler for ctrl-c
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)

	// Create new canbus client
	c, err := canusb.New(
		// Set com-port options
		canusb.OptComPort(port, baudrate),
		// Set CAN bit-rate
		canusb.OptRate(canusb.SaabPBUSRate),
	)
	if err != nil {
		log.Fatal(err)
	}

	t := time.NewTicker(10 * time.Second)

	go func() {
		time.Sleep(3 * time.Second)
		err := c.Send(&canusb.Frame{
			Identifier: "220",
			Len:        8,
			Data:       canusb.B{0x3f, 0x81, 0x01, 0x33, 0x02, 0x40, 0x00, 0x00},
		})
		if err != nil {
			log.Fatal(err)
		}
	}()

	tqRowPointer := 0x00
outer:
	for {
		select {
		case f := <-c.Chan():
			switch f.Identifier {
			case "238": //  Trionic data initialization reply
				//decodeSaabFrame(f)
				canusb.LogOut(f)
				c.Send(&canusb.Frame{
					Identifier: "240", // 240h - Trionic data query
					Len:        8,
					Data:       canusb.B{0x40, 0xA1, 0x02, 0x01, 0x0B, 0x00, 0x00, 0x00},
				})
			case "258": // 258h - Trionic data query reply
				// Got a trionic data query response
				//decodeSaabFrame(f)
				canusb.LogOut(f)
				tqRowPointer = int(f.Data[0])
				_ = tqRowPointer
				// Send ACK
				c.Send(&canusb.Frame{
					Identifier: "266",
					Len:        8,
					Data:       canusb.B{0x40, 0xA1, 0x3F, 0x81, 0x00, 0x00, 0x00, 0x00},
				})

			}

			//decodeSaabFrame(f)
		case <-t.C:
			st := c.Stats()
			log.Printf("recv: %d sent: %d errors: %d\n", st.RecvBytes, st.SentBytes, st.Errors)
		case s := <-sig:
			log.Printf("got %v, stopping CAN communication", s)
			c.Stop()
			break outer
		}
	}

	/*
			//Or if you prefer go routines
			// Start our custom frame decoder
			go func() {
				for f := range c.Chan() {
					decodeSaabFrame(f)
				}
				log.Println("CAN consumer exited")
			}()
		 	//Print some usage stats every 10 seconds
			go func() {
				for {
					time.Sleep(10 * time.Second)

				}
			}()
	*/
}

func decodeSaabFrame(f *canusb.Frame) {
	//https://pikkupossu.1g.fi/tomi/projects/p-bus/p-bus.html
	var prefix string
	var signfBit bool
	switch f.Identifier {
	case "238": // Trionic data initialization reply
		prefix = "TDI"
	case "240": //  Trionic data query
		prefix = "TDIR"
	case "258": // Trionic data query reply
		prefix = "TDQR"
	case "266": // Trionic reply acknowledgement
		prefix = "TRA"
	case "370": // Mileage
		prefix = "MLG"
	case "3A0": // Vehicle speed (MIU?)
		prefix = "MIU"
	case "1A0": // Engine information
		signfBit = true
		prefix = "ENG"
	default:
		prefix = "UNK"
	}

	if signfBit {
		log.Printf("%s> %s  %d  %08b  %X\n", prefix, f.Identifier, f.Len, f.Data[:1], f.Data[1:])
		log.Printf("%s> %s  %d  %08b  %X\n", prefix, f.Identifier, f.Len, f.Data[:1], f.Data[1:])
		return
	}

	log.Printf("in> %s> %s  %d %X\n", prefix, f.Identifier, f.Len, f.Data)
}

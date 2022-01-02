package main

import (
	"bytes"
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
	flag.StringVarP(&port, "port", "p", "", "COM-port, Windows COM#\nLinux/OSX: /dev/ttyUSB#")
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
		time.Sleep(1 * time.Second)
		c.SendFrame(0x220, canusb.B{0x3F, 0x81, 0x00, 0x11, 0x02, 0x40, 0x00, 0x00}) //init:msg
		f, err := c.WaitForFrame(0x238, 1*time.Second)
		if err != nil {
			log.Println(err)
			return
		}
		canusb.LogOut(f)
		c.SendFrame(0x240, canusb.B{0x40, 0xA1, 0x02, 0x1A, 0x90, 0x00, 0x00, 0x00})

		korv := true
		var answer []byte

		var length int
		for korv {
			f2, err := c.WaitForFrame(0x258, 1*time.Second)
			if err != nil {
				log.Println(err)
				return
			}
			canusb.LogOut(f2)
			if f2.Data[0] == 0xC3 {
				if int(f2.Data[2]) > 2 {
					length = int(f2.Data[2]) - 2
				}
				for i := 5; i < 8; i++ {
					if length > 0 {
						answer = append(answer, f2.Data[i])
					}
					length--
				}
			} else {
				for i := 0; i < 6; i++ {
					if length == 0 {
						break
					}
					answer = append(answer, f2.Data[2+i])
					length--
				}

			}
			c.SendFrame(0x266, canusb.B{0x40, 0xA1, 0x3F, f2.Data[0] & 0xBF, 0x00, 0x00, 0x00, 0x00})
			if bytes.Equal(f2.Data[:1], canusb.B{0x80}) || bytes.Equal(f2.Data[:1], canusb.B{0xC0}) {
				korv = false
			}
		}
		log.Printf("%d: %q\n", len(answer), string(answer))
	}()

outer:
	for {
		select {
		case f := <-c.Chan():
			switch f.Identifier {
			//case 0x238: //  Trionic data initialization reply
			//	canusb.LogOut(f)
			//case 0x258: // 258h - Trionic data query reply
			//canusb.LogOut(f)
			default:
				//canusb.LogOut(f)
			}
		case <-t.C:
			st := c.Stats()
			log.Printf("recv: %d sent: %d errors: %d\n", st.RecvBytes, st.SentBytes, st.Errors)
		case s := <-sig:
			log.Printf("got %v, stopping CAN communication", s)
			c.Stop()
			time.Sleep(500 * time.Millisecond)
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
	case 0x238: // Trionic data initialization reply
		prefix = "TDI"
	case 0x240: //  Trionic data query
		prefix = "TDIR"
	case 0x258: // Trionic data query reply
		prefix = "TDQR"
	case 0x266: // Trionic reply acknowledgement
		prefix = "TRA"
	case 0x370: // Mileage
		prefix = "MLG"
	case 0x3A0: // Vehicle speed (MIU?)
		prefix = "MIU"
	case 0x1A0: // Engine information
		signfBit = true
		prefix = "ENG"
	default:
		prefix = "UNK"
	}

	if signfBit {
		log.Printf("%s> 0x%x  %d  %08b  %X\n", prefix, f.Identifier, f.Len, f.Data[:1], f.Data[1:])
		//log.Printf("%s> 0x%x  %d  %08b  %X\n", prefix, f.Identifier, f.Len, f.Data[:1], f.Data[1:])
		return
	}

	log.Printf("in> %s> 0x%x  %d %X\n", prefix, f.Identifier, f.Len, f.Data)
}

package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"time"

	"github.com/avast/retry-go"
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
		err := retry.Do(
			func() error {
				go func() {
					time.Sleep(10 * time.Millisecond)
					c.SendFrame(0x220, canusb.B{0x3F, 0x81, 0x00, 0x11, 0x02, 0x40, 0x00, 0x00}) //init:msg
				}()
				_, err := c.WaitForFrame(0x238, 1200*time.Millisecond)
				if err != nil {
					return fmt.Errorf("%v", err)
				}
				return nil

			},
			retry.Attempts(20),
			retry.Delay(200*time.Millisecond),
			retry.OnRetry(func(n uint, err error) {
				log.Printf("#%d: %s\n", n, err.Error())
			}),
		)
		if err != nil {
			log.Println(err)
			return
		}

		ok := c.Tjong(0)
		if !ok {
			c.Tjong(1)
		}

		log.Println("VIN:", c.GetHeader(0x90))
		log.Println("Box HW part number:", c.GetHeader(0x91))
		log.Println("Immo Code:", c.GetHeader(0x92))
		log.Println("Software Saab part number:", c.GetHeader(0x94))
		log.Println("ECU Software version:", c.GetHeader(0x95))
		log.Println("Engine type:", c.GetHeader(0x97))
		log.Println("Tester info:", c.GetHeader(0x98))
		log.Println("Software date:", c.GetHeader(0x99))
		sig <- os.Interrupt
	}()

outer:
	for {
		select {
		case f := <-c.Chan():
			switch f.Identifier {
			//case 0x238: //  Trionic data initialization reply
			//case 0x258: // 258h - Trionic data query reply
			//case 0x258:
			//canusb.LogOut(f)
			default:
				//canusb.LogOut(f)
			}
		case <-t.C:
			st := c.Stats()
			_ = st
			//log.Printf("recv: %d sent: %d errors: %d\n", st.RecvBytes, st.SentBytes, st.Errors)
		case s := <-sig:
			log.Printf("got %v, stopping CAN communication", s)
			c.Stop()
			time.Sleep(200 * time.Millisecond)
			break outer
		}
	}
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

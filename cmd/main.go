package main

import (
	"log"
	"os"
	"os/signal"
	"strconv"
	"time"

	"github.com/roffe/canusb"
)

func init() {
	log.SetFlags(log.Lshortfile | log.LstdFlags)
}

func main() {

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)

	cfg := &canusb.Config{
		Com: canusb.ComConfig{
			Port:     "COM3",
			BaudRate: 921600,
		},
		Can: canusb.CanConfig{
			Rate: canusb.SaabPBUSRate,
		},
	}

	c, err := canusb.New(cfg)
	if err != nil {
		log.Fatal(err)
	}

	go func() {
		for d := range c.Chan() {
			dec(d)
		}
		log.Println("CAN consumer exited")
	}()

	go func() {
		for {
			time.Sleep(10 * time.Second)
			st := c.Stats()
			log.Printf("Recv: %d Sent: %d \n", st.RecvBytes, st.SentBytes)
		}
	}()

	time.Sleep(3 * time.Second)

	<-sig
	log.Println("stopping CAN communication")
	c.Stop()
}

func dec(p []byte) {
	log.Printf("%q || %X\n", string(p), p)
	if p[0] == 't' {
		if len(p) == 21 {
			addr := string(p[1:4])
			len, err := strconv.ParseUint(string(p[4:5]), 0, 8)
			data := p[5:]
			if err != nil {
				log.Fatal(err)
			}
			log.Printf("%s  %d  %08b  %s\n", addr, len, data[:1], data[1:])

			switch addr { //https://pikkupossu.1g.fi/tomi/projects/p-bus/p-bus.html
			case "6B1": // ??
			case "370": // Mileage
			case "3A0": // Vehicle speed (MIU?)
			case "1A0": // Engine information
				log.Printf("%s  %d  %08b  %s\n", addr, len, data[:1], data[1:])
			default:
				log.Printf("%s  %d  %08b  %s\n", addr, len, data[:1], data[1:])

			}
		}
		return
	}
}

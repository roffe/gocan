package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/roffe/gocan"
)

func init() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
}

func main() {
	log.Println("Starting pingpong server")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 2)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	cl, err := gocan.New(ctx, "CANUSB", &gocan.AdapterConfig{
		Port:         "COM11",
		PortBaudrate: 2000000,
		CANRate:      500,
	})
	if err != nil {
		log.Fatal(err)
	}
	defer cl.Close()

	sub := cl.Subscribe(ctx, 0x123)
	defer sub.Close()

	for {
		select {
		case s := <-sigChan:
			log.Println("Shutting down server, ", s)
			cancel()
		case frame := <-sub.Chan():
			log.Println(frame.String())
			err := cl.Send(0x124, []byte("pong"), gocan.Outgoing)
			if err != nil {
				log.Println(err)
			}
		case <-ctx.Done():
			return
		}
	}
}

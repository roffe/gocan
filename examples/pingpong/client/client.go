package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/roffe/gocan"
	"github.com/roffe/gocan/adapter"
)

func init() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	log.Println(adapter.List())
}

func main() {
	log.Println("Starting pingpong client")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 2)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	dev, err := adapter.New("Canlib #0 Kvaser Leaf Light v2", &gocan.AdapterConfig{
		CANRate: 500,
	})
	if err != nil {
		log.Fatal(err)
	}

	cl, err := gocan.NewClient(ctx, dev)
	if err != nil {
		log.Fatal(err)
	}
	defer cl.Close()

	t := time.NewTicker(1 * time.Second)
	defer t.Stop()

	for {
		select {
		case s := <-sigChan:
			log.Println("Shutting down client, ", s)
			cancel()
		case <-t.C:
			frame, err := cl.SendAndWait(ctx, gocan.NewFrame(0x123, []byte("ping"), gocan.ResponseRequired), 100*time.Millisecond, 0x124)
			if err != nil {
				log.Println(err)
			}
			log.Println(frame.String())
		case <-ctx.Done():
			return
		}
	}
}

package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/roffe/gocan"
)

func init() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
}

func main() {
	log.Println("Starting pingpong client")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 2)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	cl, err := gocan.New(ctx, "CANlib #0 Kvaser Leaf Light v2", &gocan.AdapterConfig{
		CANRate: 500,
	})
	if err != nil {
		log.Fatal(err)
	}
	defer cl.Close()

	t := time.NewTicker(1 * time.Second)
	defer t.Stop()

	onMsg := func(f *gocan.CANFrame) {
		log.Println("Got frame: ", f.String())
	}

	subFunc := cl.SubscribeFunc(ctx, onMsg, 0x124)
	defer subFunc.Close()

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

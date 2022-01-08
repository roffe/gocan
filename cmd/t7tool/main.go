package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"time"

	"github.com/roffe/canusb/cmd/t7tool/cmd"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel() // Setup interupt handler for ctrl-c
	quitChan := make(chan os.Signal, 1)
	signal.Notify(quitChan, os.Interrupt)
	go func() {
		s := <-quitChan
		log.Printf("got %v, exiting", s)
		cancel()
		// Failsafe if there is deadlocks
		<-time.After(45 * time.Second)
		log.Fatal("took to long to shutdown, forcefully exiting")
	}()
	cmd.Execute(ctx)
}

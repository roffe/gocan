package main

import (
	"context"
	"log"

	"github.com/roffe/gocan/cmd/gocan/gui"
)

func init() {
	log.SetFlags(log.Lshortfile | log.LstdFlags)
}

func main() {
	gui.Run(context.TODO())
}

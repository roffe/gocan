package main

import (
	_ "embed"
	"log"

	"github.com/roffe/gocan/cmd/ledenabler/window"
)

func init() {
	log.SetFlags(log.Lshortfile | log.LstdFlags)
}

func main() {
	mw := window.NewMainWindow()
	mw.Window.ShowAndRun()
}

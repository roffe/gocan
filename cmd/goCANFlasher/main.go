package main

import (
	"context"
	_ "embed"
	"log"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"github.com/roffe/gocan/cmd/goCANFlasher/gui"
)

//go:embed ECU.png
var appIconBytes []byte

var appIcon = fyne.NewStaticResource("ecu.png", appIconBytes)

func init() {
	log.SetFlags(log.Lshortfile | log.LstdFlags)
}

func main() {
	a := app.NewWithID("GoCANFlasher")
	a.Settings().SetTheme(&MyTheme{})
	a.SetIcon(appIcon)
	gui.ShowAndRun(context.TODO(), a)
}

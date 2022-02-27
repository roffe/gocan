package main

import (
	"context"
	_ "embed"
	"log"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"github.com/roffe/gocan/cmd/goCANFlasher/gui"
)

//go:embed trionicCanFlasher.png
var trionicCanFlasherIcon []byte

var appIcon = fyne.NewStaticResource("trionicCanFlasherIcon.ico", trionicCanFlasherIcon)

func init() {
	log.SetFlags(log.Lshortfile | log.LstdFlags)
}

func main() {
	a := app.NewWithID("GoCANFlasher")
	a.Settings().SetTheme(&gocanTheme{})
	a.SetIcon(appIcon)
	gui.Run(context.TODO(), a)
}

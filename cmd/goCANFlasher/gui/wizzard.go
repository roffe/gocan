package gui

import (
	_ "embed"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"
	"github.com/roffe/gocan/pkg/ecu"
)

//go:embed ng900.png
var ng900 []byte

//go:embed og95.png
var og95 []byte

//go:embed ng93.png
var ng93 []byte

//go:embed clone.png
var clone []byte

type wizzard struct {
	app    fyne.App
	mw     *mainWindow
	window fyne.Window
}

func newWizzard(a fyne.App, mw *mainWindow) *wizzard {
	w := a.NewWindow("Wizzard")
	wm := &wizzard{
		app:    a,
		mw:     mw,
		window: w,
	}
	w.SetCloseIntercept(func() {
		w.Hide()
	})
	w.SetContent(wm.selectCar())
	w.Resize(fyne.NewSize(1050, 150))
	return wm
}

var (
	x900 = &canvas.Image{
		Resource: &fyne.StaticResource{
			StaticName:    "ng900.png",
			StaticContent: ng900},
	}

	x95 = &canvas.Image{
		Resource: &fyne.StaticResource{
			StaticName:    "og95.png",
			StaticContent: og95},
	}

	x93 = &canvas.Image{
		Resource: &fyne.StaticResource{
			StaticName:    "ng93.png",
			StaticContent: ng93},
	}

	xclone = &canvas.Image{
		Resource: &fyne.StaticResource{
			StaticName:    "clone.png",
			StaticContent: clone},
	}
)

func (w *wizzard) newTappableImage(img *canvas.Image, btnFunc func()) fyne.CanvasObject {
	img.ScaleMode = canvas.ImageScaleFastest
	img.FillMode = canvas.ImageFillContain
	img.SetMinSize(fyne.NewSize(300, 150))
	openButton := widget.NewButton("", btnFunc)
	box := container.NewPadded(img, openButton)
	return box
}

func (w *wizzard) selectOperation() fyne.CanvasObject {
	cl := w.newTappableImage(xclone, func() {
		w.window.Hide()
		w.mw.ecuClone()
	})
	cloneText := widget.NewLabel("Clone ECU")
	cloneText.Alignment = fyne.TextAlignCenter
	return container.New(
		layout.NewVBoxLayout(),
		container.NewVBox(
			cl,
			cloneText,
			layout.NewSpacer(),
			widget.NewButton("Back", func() {
				w.window.SetContent(w.selectCar())
			}),
		),
	)
}

func (w *wizzard) selectCar() fyne.CanvasObject {

	sel900 := func() {
		w.mw.setECU(ecu.Trionic5)
		w.window.SetContent(w.selectOperation())
	}

	sel95 := func() {
		w.mw.setECU(ecu.Trionic7)
		w.window.SetContent(w.selectOperation())
	}

	sel93 := func() {
		w.mw.setECU(ecu.Trionic8)
		w.window.SetContent(w.selectOperation())
	}

	n900 := w.newTappableImage(x900, sel900)
	n95 := w.newTappableImage(x95, sel95)
	n93 := w.newTappableImage(x93, sel93)

	ng900text := widget.NewLabel("Saab 900II\nSaab 9000\nSaab 9-3I (T5)")
	ng900text.Alignment = fyne.TextAlignCenter

	o95text := widget.NewLabel("Saab 9-5\nSaab 9-3I (T7)")
	o95text.Alignment = fyne.TextAlignCenter

	n93text := widget.NewLabel("Saab 9-3II (T8)")
	n93text.Alignment = fyne.TextAlignCenter

	return container.New(
		layout.NewVBoxLayout(),
		container.New(
			layout.NewHBoxLayout(),
			layout.NewSpacer(),
			widget.NewLabel("~~ Select Car ~~"),
			layout.NewSpacer(),
		),
		container.New(
			layout.NewHBoxLayout(),
			layout.NewSpacer(),
			container.NewVBox(
				n900,
				ng900text,
			),
			layout.NewSpacer(),
			container.NewVBox(
				n95,
				o95text,
			),
			layout.NewSpacer(),
			container.NewVBox(
				n93,
				n93text,
			),
			layout.NewSpacer(),
		),
	)
}

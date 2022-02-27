package gui

import (
	_ "embed"
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"
)

//go:embed ng900.png
var ng900 []byte

//go:embed og95.png
var og95 []byte

//go:embed ng93.png
var ng93 []byte

type wizzard struct {
	app     fyne.App
	parrent fyne.Window
	window  fyne.Window
}

func newWizzard(a fyne.App, pw fyne.Window) *wizzard {
	w := a.NewWindow("Wizzard")
	wm := &wizzard{
		app:     a,
		parrent: pw,
		window:  w,
	}
	w.SetCloseIntercept(func() {
		w.Hide()
	})
	w.SetContent(wm.selectCar())
	w.Resize(fyne.NewSize(1050, 300))
	w.Canvas().Refresh(wm.selectCar())
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
)

func (w *wizzard) newTappableImage(img *canvas.Image) fyne.CanvasObject {
	img.ScaleMode = canvas.ImageScaleFastest
	img.FillMode = canvas.ImageFillContain

	img.SetMinSize(fyne.NewSize(300, 200))

	openButton := widget.NewButton("", func() {
		fmt.Println("Image clicked...")
		w.window.SetContent(w.selectOperation())
	})
	box := container.NewPadded(img, openButton)
	return box
}

func (w *wizzard) selectOperation() fyne.CanvasObject {
	return container.New(
		layout.NewMaxLayout(),
		container.NewCenter(
			widget.NewLabel("Kiss"),
		),
	)
}

func (w *wizzard) selectCar() fyne.CanvasObject {
	n900 := w.newTappableImage(x900)
	n95 := w.newTappableImage(x95)
	n93 := w.newTappableImage(x93)

	ng900text := widget.NewLabel("Saab 900II\nSaab 9000\nSaab 9-3I (T5)")
	ng900text.Alignment = fyne.TextAlignCenter

	o95text := widget.NewLabel("Saab 9-5\nSaab 9-3I (T7)")
	o95text.Alignment = fyne.TextAlignCenter

	n93text := widget.NewLabel("Saab 9-3II (T8)")
	n93text.Alignment = fyne.TextAlignCenter

	return container.New(
		layout.NewMaxLayout(),
		container.NewHBox(
			layout.NewSpacer(),
			container.NewVBox(
				layout.NewSpacer(),
				n900,
				layout.NewSpacer(),
				ng900text,
				layout.NewSpacer(),
			),
			layout.NewSpacer(),
			container.NewVBox(
				layout.NewSpacer(),
				n95,
				layout.NewSpacer(),
				o95text,
				layout.NewSpacer(),
			),
			layout.NewSpacer(),
			container.NewVBox(
				layout.NewSpacer(),
				n93,
				layout.NewSpacer(),
				n93text,
				layout.NewSpacer(),
			),
			layout.NewSpacer(),
		),
	)
}

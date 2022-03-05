package window

import (
	_ "embed"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"
)

//go:embed ECU.png
var icon []byte
var iconRes = fyne.NewStaticResource("ECU.png", icon)

//go:embed ng93.png
var ng93 []byte
var ng93res = fyne.NewStaticResource("ng93.png", ng93)
var ng93img = canvas.NewImageFromResource(ng93res)

func init() {
	ng93img.ScaleMode = canvas.ImageScaleSmooth
	ng93img.FillMode = canvas.ImageFillContain
	ng93img.SetMinSize(fyne.NewSize(300, 221))
}

type Main struct {
	App      fyne.App
	Window   fyne.Window
	Settings *Settings

	Output     *widget.List
	outputData binding.StringList

	Progress *widget.ProgressBar
	Btn      *widget.Button
}

func NewMainWindow() *Main {
	a := app.NewWithID("Ledenabler")
	w := a.NewWindow("TrionicTuning Ledenabler")

	w.SetIcon(iconRes)
	w.Resize(fyne.NewSize(400, 250))

	a.Settings().SetTheme(&gocanTheme{})

	mw := &Main{
		App:        a,
		Window:     w,
		Settings:   NewSettingsWindow(a, w),
		outputData: binding.NewStringList(),
	}

	w.SetCloseIntercept(func() {
		mw.Settings.Window.Close()
		w.Close()
	})

	w.SetContent(mw.Layout())
	return mw
}

func (m *Main) enableLED() {
	m.Window.Resize(fyne.NewSize(400, 500))
	m.Output.Show()
	m.Btn.Disable()
	m.Progress.Show()
	defer m.Btn.Enable()

	m.outputData.Append("Enabling LED")
	for i := 0; i <= 10; i++ {
		m.outputData.Append("...")
		m.Output.ScrollToBottom()
		time.Sleep(100 * time.Millisecond)
		m.Progress.SetValue(float64(i * 10))
	}

	m.outputData.Append("Done")
	m.Output.ScrollToBottom()
	m.Progress.Hide()
}

func (m *Main) Layout() fyne.CanvasObject {
	m.Btn = widget.NewButton("Enable LED", m.enableLED)
	m.Btn.Icon = ng93res

	m.Output = widget.NewListWithData(
		m.outputData,
		func() fyne.CanvasObject {
			w := widget.NewLabel("")
			w.Alignment = fyne.TextAlignCenter
			w.TextStyle.Monospace = true
			return w
		},
		func(item binding.DataItem, obj fyne.CanvasObject) {
			i := item.(binding.String)
			txt, err := i.Get()
			if err != nil {
				panic(err)
			}
			obj.(*widget.Label).SetText(txt)
		},
	)
	m.Output.Hide()

	m.Progress = widget.NewProgressBar()
	m.Progress.Hide()
	m.Progress.Max = 100
	btn2 := widget.NewButton("Settings", func() {
		m.Settings.Window.Show()
	})

	footer := container.NewVBox(
		m.Progress,
		container.NewHSplit(
			btn2,
			m.Btn,
		),
	)

	content := container.New(layout.NewBorderLayout(
		ng93img,
		footer,
		nil,
		nil,
	),
		ng93img,
		footer,
		m.Output,
	)
	return content
}

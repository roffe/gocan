package window

import "fyne.io/fyne/v2"

type Settings struct {
	App    fyne.App
	Window fyne.Window
	Parent fyne.Window
}

func NewSettingsWindow(app fyne.App, parent fyne.Window) *Settings {
	w := app.NewWindow("Settings")
	w.SetIcon(iconRes)
	w.Resize(fyne.NewSize(500, 500))
	w.SetCloseIntercept(func() {
		w.Hide()
	})
	return &Settings{
		App:    app,
		Window: w,
		Parent: parent,
	}
}

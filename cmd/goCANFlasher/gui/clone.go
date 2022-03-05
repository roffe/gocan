package gui

import (
	"time"

	sdialog "github.com/sqweek/dialog"
)

func (m *mainWindow) ecuClone() {
	if !m.checkSelections() {
		return
	}
	m.progressBar.Max = 100
	m.progressBar.SetValue(0)
	m.disableButtons()
	defer m.enableButtons()

	m.output("Cloning old ECU")
	for i := 0; i < 50; i++ {
		m.progressBar.SetValue(m.progressBar.Value + 1)
		time.Sleep(100 * time.Millisecond)
	}
	m.output("Swap ECU")
	ok := sdialog.Message("%s", "Remove old ECU and connect new\nPlease wait 20 seconds after connecting new ECU before pressing Yes\nContinue?").Title("Plug in new ECU").YesNo()
	if !ok {
		m.output("Clone aborted by user")
		m.progressBar.SetValue(0)
		return
	}

	m.output("Flashing new ECU")
	for i := 0; i < 50; i++ {
		m.progressBar.SetValue(m.progressBar.Value + 1)
		time.Sleep(100 * time.Millisecond)
	}
	m.output("Done flashing new ECU")

	m.output("ECU clone done")

	m.progressBar.SetValue(100)
}

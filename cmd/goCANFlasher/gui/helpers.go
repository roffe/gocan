package gui

import "github.com/roffe/gocan/pkg/ecu"

func (mw *mainWindow) setECU(t ecu.Type) {
	state.ecuType = t
	mw.ecuList.SetSelected(t.String())
}

package gui

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"
	"github.com/roffe/gocan/adapter"
	"github.com/roffe/gocan/pkg/ecu"
	"github.com/roffe/gocan/pkg/ecu/t5"
	"github.com/roffe/gocan/pkg/ecu/t7"
	"github.com/roffe/gocan/pkg/ecu/t8"
)

const eas = "KW"

type mainWindow struct {
	app           fyne.App
	window        fyne.Window
	wizzardWindow *wizzard

	log *widget.List

	wizzardBTN *widget.Button

	ecuList     *widget.Select
	adapterList *widget.Select
	portList    *widget.Select
	speedList   *widget.Select

	dtcBTN     *widget.Button
	infoBTN    *widget.Button
	dumpBTN    *widget.Button
	sramBTN    *widget.Button
	flashBTN   *widget.Button
	refreshBTN *widget.Button

	progressBar *widget.ProgressBar
}

var keyhandler = bytes.NewBuffer(nil)

func newMainWindow(a fyne.App, w fyne.Window) *mainWindow {
	m := &mainWindow{
		app:         a,
		window:      w,
		log:         createLogList(),
		progressBar: widget.NewProgressBar(),
	}

	m.wizzardWindow = newWizzard(a, m)

	w.Canvas().SetOnTypedKey(m.onTypedKey)
	w.SetCloseIntercept(m.closeHandler)
	m.createSelects()
	m.createButtons()
	w.SetContent(m.layout())
	w.Resize(fyne.NewSize(1024, 450))
	return m
}

func (m *mainWindow) onTypedKey(ke *fyne.KeyEvent) {
	keyhandler.WriteString(string(ke.Name))
	if keyhandler.String() == ter+eas {
		m.output(string(xor(cmd1)))
		keyhandler.Reset()
	}
	if keyhandler.Len() > 4 {
		keyhandler.Reset()
	}
}

func createLogList() *widget.List {
	return widget.NewListWithData(
		listData,
		func() fyne.CanvasObject {
			w := widget.NewLabel("")
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
}

func (m *mainWindow) layout() *container.Split {

	left := container.New(layout.NewMaxLayout(), m.log)
	right := container.NewVBox(
		widget.NewLabel(""),
		m.wizzardBTN,
		widget.NewLabel(""),
		m.ecuList,
		m.adapterList,
		m.portList,
		m.speedList,
		layout.NewSpacer(),
		m.infoBTN,
		m.dtcBTN,
		m.dumpBTN,
		m.sramBTN,
		m.flashBTN,
		m.refreshBTN,
	)

	split := container.NewHSplit(left, right)
	split.Offset = 0.8

	view := container.NewVSplit(split, m.progressBar)
	view.Offset = 1
	return view
}

func (m *mainWindow) createButtons() {
	m.wizzardBTN = widget.NewButton("Wizzard", m.wizzard)
	m.dtcBTN = widget.NewButton("Read DTC", m.readDTC)
	m.infoBTN = widget.NewButton("Info", m.ecuInfo)
	m.sramBTN = widget.NewButton("Dump SRAM", m.dumpSRAM)
	m.dumpBTN = widget.NewButton("Dump", m.ecuDump)
	m.flashBTN = widget.NewButton("Flash", m.ecuFlash)
	m.refreshBTN = widget.NewButton("Refresh Ports", m.refreshPorts)
}

func (m *mainWindow) wizzard() {
	m.wizzardWindow.window.Show()
}

func (m *mainWindow) createSelects() {
	m.ecuList = widget.NewSelect([]string{"Trionic 5", "Trionic 7", "Trionic 8", "Trionic 8 MCP"}, func(s string) {
		index := m.ecuList.SelectedIndex()
		state.ecuType = ecu.Type(index + 1)
		switch state.ecuType {
		case ecu.Trionic5:
			state.canRate = t5.PBusRate
			m.dtcBTN.Hide()
		case ecu.Trionic7:
			m.dtcBTN.Hide()
			state.canRate = t7.PBusRate
		case ecu.Trionic8:
			m.dtcBTN.Show()
			state.canRate = t8.PBusRate
		}

		m.app.Preferences().SetFloat("canrate", state.canRate)
		m.app.Preferences().SetInt("ecu", index)

	})

	m.adapterList = widget.NewSelect(adapter.List(), func(s string) {
		state.adapter = s
		m.app.Preferences().SetString("adapter", s)
	})

	m.portList = widget.NewSelect(m.listPorts(), func(s string) {
		state.port = s
		m.app.Preferences().SetString("port", s)
	})

	m.speedList = widget.NewSelect(speeds(), func(s string) {
		speed, err := strconv.Atoi(s)
		if err != nil {
			m.output("failed to set port speed: " + err.Error())
		}
		state.portBaudrate = speed
		m.app.Preferences().SetString("portSpeed", s)
	})

	m.ecuList.PlaceHolder = "Select ECU"
	m.adapterList.PlaceHolder = "Select Adapter"
	m.portList.PlaceHolder = "Select Port"
	m.speedList.PlaceHolder = "Select Speed"
}

func (m *mainWindow) refreshPorts() {
	m.portList.Options = m.listPorts()
	m.portList.Refresh()
}

func (m *mainWindow) checkSelections() bool {
	var out strings.Builder
	if m.ecuList.SelectedIndex() < 0 {
		out.WriteString("ECU type\n")
	}
	if m.adapterList.SelectedIndex() < 0 {
		out.WriteString("Adapter\n")
	}

	if m.speedList.SelectedIndex() < 0 {
		out.WriteString("Speed\n")
	}
	if out.Len() > 0 {
		//sdialog.Message("Please set the following options:\n%s", out.String()).Title("error").Error()
		dialog.ShowError(fmt.Errorf("Please set the following options:\n%s", out.String()), m.window) //lint:ignore ST1005 ignore this error
		return false

	}
	return true
}

func (m *mainWindow) disableButtons() {
	m.ecuList.Disable()
	m.adapterList.Disable()
	m.portList.Disable()
	m.speedList.Disable()

	m.dtcBTN.Disable()
	m.infoBTN.Disable()
	m.dumpBTN.Disable()
	m.sramBTN.Disable()
	m.flashBTN.Disable()
}

func (m *mainWindow) enableButtons() {
	m.ecuList.Enable()
	m.adapterList.Enable()
	m.portList.Enable()
	m.speedList.Enable()

	m.dtcBTN.Enable()
	m.infoBTN.Enable()
	m.dumpBTN.Enable()
	m.sramBTN.Enable()
	m.flashBTN.Enable()
}

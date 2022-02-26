package gui

import (
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"
	"github.com/roffe/gocan/pkg/ecu"
	"github.com/roffe/gocan/pkg/t5"
	"github.com/roffe/gocan/pkg/t7"
	"github.com/roffe/gocan/pkg/t8"
	sdialog "github.com/sqweek/dialog"
)

type mainWindow struct {
	app    fyne.App
	window fyne.Window

	log *widget.List

	ecuList     *widget.Select
	adapterList *widget.Select
	portList    *widget.SelectEntry
	speedList   *widget.Select

	refreshBTN *widget.Button
	infoBTN    *widget.Button
	dumpBTN    *widget.Button
	flashBTN   *widget.Button

	progressBar *widget.ProgressBar
}

func newMainWindow(a fyne.App, w fyne.Window) *mainWindow {
	m := &mainWindow{
		app:    a,
		window: w,

		log:         createLogList(),
		progressBar: widget.NewProgressBar(),
	}
	m.createSelects()
	m.createButtons()
	return m
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
		m.ecuList,
		m.adapterList,
		m.portList,
		m.speedList,
		layout.NewSpacer(),
		m.infoBTN,
		m.dumpBTN,
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
	m.refreshBTN = widget.NewButton("Refresh Ports", m.refreshPorts)
	m.infoBTN = widget.NewButton("Info", m.ecuInfo)
	m.dumpBTN = widget.NewButton("Dump", m.ecuDump)
	m.flashBTN = widget.NewButton("Flash", m.ecuFlash)
}

func (m *mainWindow) createSelects() {
	m.ecuList = widget.NewSelect([]string{"Trionic 5", "Trionic 7", "Trionic 8"}, func(s string) {
		index := m.ecuList.SelectedIndex()
		state.ecuType = ecu.Type(index + 1)
		switch state.ecuType {
		case ecu.Trionic5:
			state.canRate = t5.PBusRate
		case ecu.Trionic7:
			state.canRate = t7.PBusRate
		case ecu.Trionic8:
			state.canRate = t8.PBusRate
		}
		m.app.Preferences().SetFloat("canrate", state.canRate)
		m.app.Preferences().SetInt("ecu", index)

	})

	m.adapterList = widget.NewSelect(adapters(), func(s string) {
		state.adapter = s
		m.app.Preferences().SetString("adapter", s)
	})

	state.portList = m.ports()

	m.portList = widget.NewSelectEntry(state.portList)
	m.portList.OnChanged = func(s string) {
		state.port = s
		m.app.Preferences().SetString("port", s)
	}

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
	m.portList.SetOptions(m.ports())
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

	//if mw.portList.SelectedIndex() < 0 {
	//	out.WriteString("Port\n")
	//}

	if m.speedList.SelectedIndex() < 0 {
		out.WriteString("Speed\n")
	}
	if out.Len() > 0 {
		sdialog.Message("Please set the following options:\n%s", out.String()).Title("error").Error()
		return false
	}
	return true
}

func (m *mainWindow) disableButtons() {
	m.infoBTN.Disable()
	m.dumpBTN.Disable()
	m.flashBTN.Disable()
}

func (m *mainWindow) enableButtons() {
	m.infoBTN.Enable()
	m.dumpBTN.Enable()
	m.flashBTN.Enable()
}

package gui

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"
	"github.com/roffe/gocan/pkg/ecu"
	"github.com/roffe/gocan/pkg/t5"
	"github.com/roffe/gocan/pkg/t7"
	"github.com/roffe/gocan/pkg/t8"
	sdialog "github.com/sqweek/dialog"
	"go.bug.st/serial/enumerator"
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

type appState struct {
	ecuType   ecu.Type
	canRate   float64
	adapter   string
	port      string
	portSpeed int
	portList  []string
}

var (
	mw       *mainWindow
	listData = binding.NewStringList()
	state    *appState
)

func init() {
	state = &appState{}
}

func Run(ctx context.Context) {
	a := app.NewWithID("GoCANFlasher")
	a.Settings().SetTheme(&gocanTheme{})

	w := a.NewWindow("GoCANFlasher")
	w.Resize(fyne.NewSize(900, 500))

	mw = &mainWindow{
		app:    a,
		window: w,

		log: widget.NewListWithData(
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
				//obj.(*widget.Entry).SetText(txt)
				obj.(*widget.Label).SetText(txt)
				//obj.(*widget.RichText).ParseMarkdown(txt)
			},
		),
		refreshBTN: widget.NewButton("Refresh Ports", refreshPorts),
		infoBTN:    widget.NewButton("Info", ecuInfo),
		dumpBTN:    widget.NewButton("Dump", ecuDump),
		flashBTN:   widget.NewButton("Flash", ecuFlash),

		progressBar: widget.NewProgressBar(),
	}

	mw.ecuList = widget.NewSelect([]string{"Trionic 5", "Trionic 7", "Trionic 8"}, func(s string) {
		index := mw.ecuList.SelectedIndex()
		state.ecuType = ecu.Type(index + 1)
		switch state.ecuType {
		case ecu.Trionic5:
			state.canRate = t5.PBusRate
		case ecu.Trionic7:
			state.canRate = t7.PBusRate
		case ecu.Trionic8:
			state.canRate = t8.PBusRate
		}
		a.Preferences().SetFloat("canrate", state.canRate)
		a.Preferences().SetInt("ecu", index)

	})

	mw.adapterList = widget.NewSelect(adapters(), func(s string) {
		state.adapter = s
		a.Preferences().SetString("adapter", s)
	})

	state.portList = ports()

	mw.portList = widget.NewSelectEntry(state.portList)
	mw.portList.OnChanged = func(s string) {
		state.port = s
		a.Preferences().SetString("port", s)
	}

	mw.speedList = widget.NewSelect(speeds(), func(s string) {
		speed, err := strconv.Atoi(s)
		if err != nil {
			output("failed to set port speed: " + err.Error())
		}
		state.portSpeed = speed
		a.Preferences().SetString("portSpeed", s)
	})

	mw.ecuList.PlaceHolder = "Select ECU"
	mw.adapterList.PlaceHolder = "Select Adapter"
	mw.portList.PlaceHolder = "Select Port"
	mw.speedList.PlaceHolder = "Select Speed"

	left := container.New(layout.NewMaxLayout(), mw.log)

	right := container.NewVBox(
		widget.NewLabel(""),
		mw.ecuList,
		//bus,
		mw.adapterList,
		mw.portList,
		mw.speedList,

		layout.NewSpacer(),
		mw.infoBTN,
		mw.dumpBTN,
		mw.flashBTN,
		mw.refreshBTN,
	)

	split := container.NewHSplit(left, right)
	split.Offset = 0.8

	view := container.NewVSplit(split, mw.progressBar)
	view.Offset = 1

	w.SetContent(view)

	go func() {
		<-ctx.Done()
		w.Close()
	}()

	loadPreferences()

	go func() {
		time.Sleep(10 * time.Millisecond)
		output("Done detecting ports")
	}()

	w.ShowAndRun()
}

func refreshPorts() {
	mw.portList.SetOptions(ports())
	mw.portList.Refresh()
}

func checkSelections() bool {
	var out strings.Builder
	if mw.ecuList.SelectedIndex() < 0 {
		out.WriteString("ECU type\n")
	}
	if mw.adapterList.SelectedIndex() < 0 {
		out.WriteString("Adapter\n")
	}

	//if mw.portList.SelectedIndex() < 0 {
	//	out.WriteString("Port\n")
	//}
	if mw.speedList.SelectedIndex() < 0 {
		out.WriteString("Speed\n")
	}
	if out.Len() > 0 {
		sdialog.Message("Please set the following options:\n%s", out.String()).Title("error").Error()
		return false
	}
	return true
}

func loadPreferences() {
	state.canRate = mw.app.Preferences().FloatWithFallback("canrate", 500)
	mw.ecuList.SetSelectedIndex(mw.app.Preferences().IntWithFallback("ecu", 0))
	mw.adapterList.SetSelected(mw.app.Preferences().StringWithFallback("adapter", "Canusb"))
	state.port = mw.app.Preferences().String("port")
	mw.portList.PlaceHolder = state.port
	mw.portList.Refresh()
	mw.speedList.SetSelected(mw.app.Preferences().StringWithFallback("portSpeed", "115200"))
}

func output(s string) {
	var text string
	if s != "" {
		text = fmt.Sprintf("%s - %s\n", time.Now().Format("15:04:05.000"), s)
	}
	//logData = append(logData, text)
	listData.Append(text)
	mw.log.Refresh()
	mw.log.ScrollToBottom()
}

func adapters() []string {
	return []string{"Canusb", "OBDLinkSX"}
}

func speeds() []string {
	var out []string
	l := []int{9600, 19200, 38400, 57600, 115200, 230400, 460800, 921600, 1000000, 2000000}
	for _, ll := range l {
		out = append(out, strconv.Itoa(ll))
	}
	return out
}

func disableButtons() {
	mw.infoBTN.Disable()
	mw.dumpBTN.Disable()
	mw.flashBTN.Disable()
}

func enableButtons() {
	mw.infoBTN.Enable()
	mw.dumpBTN.Enable()
	mw.flashBTN.Enable()
}

func ports() []string {
	var portsList []string
	ports, err := enumerator.GetDetailedPortsList()
	if err != nil {
		output(err.Error())
		return []string{}
	}
	if len(ports) == 0 {
		output("No serial ports found!")
		return []string{}
	}
	for _, port := range ports {
		output(fmt.Sprintf("Found port: %s", port.Name))
		if port.IsUSB {
			output(fmt.Sprintf("  USB ID     %s:%s", port.VID, port.PID))
			output(fmt.Sprintf("  USB serial %s", port.SerialNumber))
			portsList = append(portsList, port.Name)
		}
	}
	return portsList
}

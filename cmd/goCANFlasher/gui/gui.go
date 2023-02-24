package gui

import (
	"context"
	"fmt"
	"strconv"
	"time"

	_ "embed"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/data/binding"
	"github.com/roffe/gocan/adapter/j2534"
	"github.com/roffe/gocan/pkg/ecu"
	sdialog "github.com/sqweek/dialog"
	"go.bug.st/serial/enumerator"
)

type appState struct {
	ecuType      ecu.Type
	canRate      float64
	adapter      string
	port         string
	portBaudrate int
	portList     []string
	inprogress   bool
}

var (
	listData = binding.NewStringList()
	state    *appState
)

func init() {
	state = &appState{}
}

func ShowAndRun(ctx context.Context, a fyne.App) {
	w := a.NewWindow("GoCANFlasher")

	mw := newMainWindow(a, w)

	go func() {
		<-ctx.Done()
		time.Sleep(500 * time.Millisecond)
		w.Close()
	}()

	mw.loadPreferences()

	go func() {
		time.Sleep(10 * time.Millisecond)
		mw.output("Done detecting ports")
	}()

	w.ShowAndRun()
}

func (m *mainWindow) closeHandler() {
	if state.inprogress {
		if sdialog.Message("Are you sure, operation still in progress").Title("In progress").YesNo() {
			m.window.Close()
		}
		return
	}
	m.wizzardWindow.window.Close()
	m.window.Close()
}

func (m *mainWindow) loadPreferences() {
	state.canRate = m.app.Preferences().FloatWithFallback("canrate", 500)
	m.ecuList.SetSelectedIndex(m.app.Preferences().IntWithFallback("ecu", 0))
	m.adapterList.SetSelected(m.app.Preferences().StringWithFallback("adapter", "Canusb"))
	state.port = m.app.Preferences().String("port")
	m.portList.PlaceHolder = state.port
	m.portList.Refresh()
	m.speedList.SetSelected(m.app.Preferences().StringWithFallback("portSpeed", "115200"))
}

func speeds() []string {
	var out []string
	l := []int{9600, 19200, 38400, 57600, 115200, 230400, 460800, 921600, 1000000, 2000000}
	for _, ll := range l {
		out = append(out, strconv.Itoa(ll))
	}
	return out
}

func (m *mainWindow) listPorts() []string {
	var portsList []string
	ports, err := enumerator.GetDetailedPortsList()
	if err != nil {
		m.output(err.Error())
		return []string{}
	}
	if len(ports) == 0 {
		m.output("No serial ports found!")
		//return []string{}
	}
	for _, port := range ports {
		m.output(fmt.Sprintf("Found port: %s", port.Name))
		if port.IsUSB {
			m.output(fmt.Sprintf("  USB ID     %s:%s", port.VID, port.PID))
			m.output(fmt.Sprintf("  USB serial %s", port.SerialNumber))
			portsList = append(portsList, port.Name)
		}
	}

	dlls := j2534.FindDLLs()
	for _, dll := range dlls {
		m.output(fmt.Sprintf("J2534 DLL: %s", dll.Name))
		//portsList = append(portsList, filepath.Base(dll.FunctionLibrary))
		portsList = append(portsList, dll.Name)
	}

	state.portList = portsList
	return portsList
}

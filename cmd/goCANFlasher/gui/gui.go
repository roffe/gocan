package gui

import (
	"context"
	"fmt"
	"strconv"
	"time"

	_ "embed"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/data/binding"
	"github.com/roffe/gocan/pkg/ecu"
	"go.bug.st/serial/enumerator"
)

type appState struct {
	ecuType      ecu.Type
	canRate      float64
	adapter      string
	port         string
	portBaudrate int
	portList     []string
}

var (
	listData = binding.NewStringList()
	state    *appState
)

func init() {
	state = &appState{}
}

func Run(ctx context.Context, a fyne.App) {
	w := a.NewWindow("GoCANFlasher")
	w.Resize(fyne.NewSize(900, 500))

	mw := newMainWindow(a, w)

	w.SetContent(mw.layout())

	go func() {
		<-ctx.Done()
		w.Close()
	}()

	mw.loadPreferences()

	go func() {
		time.Sleep(10 * time.Millisecond)
		mw.output("Done detecting ports")
	}()

	w.ShowAndRun()
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

func (m *mainWindow) ports() []string {
	var portsList []string
	ports, err := enumerator.GetDetailedPortsList()
	if err != nil {
		m.output(err.Error())
		return []string{}
	}
	if len(ports) == 0 {
		m.output("No serial ports found!")
		return []string{}
	}
	for _, port := range ports {
		m.output(fmt.Sprintf("Found port: %s", port.Name))
		if port.IsUSB {
			m.output(fmt.Sprintf("  USB ID     %s:%s", port.VID, port.PID))
			m.output(fmt.Sprintf("  USB serial %s", port.SerialNumber))
			portsList = append(portsList, port.Name)
		}
	}
	return portsList
}
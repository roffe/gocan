package gui

import (
	"context"
	"math"
	"os"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"github.com/roffe/gocan/pkg/ecu"
	sdialog "github.com/sqweek/dialog"
)

func xor(data []byte) []byte {
	out := make([]byte, len(data))
	for i, b := range data {
		out[i] = b ^ 0xFF
	}
	return out
}

func addSuffix(s, suffix string) string {
	if !strings.HasSuffix(s, suffix) {
		return s + suffix
	}
	return s
}

func (m *mainWindow) ecuDump() {
	if !m.checkSelections() {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 900*time.Second)

	filename, err := sdialog.File().Filter("Bin file", "bin").Title("Save bin file").Save()
	if err != nil {
		m.output(err.Error())
		cancel()
		return
	}
	filename = addSuffix(filename, ".bin")
	m.progressBar.SetValue(0)

	go func() {
		state.inprogress = true
		defer func() {
			state.inprogress = false
		}()

		m.disableButtons()
		defer m.enableButtons()
		defer cancel()

		c, err := m.initCAN(ctx)
		if err != nil {
			m.output(err.Error())
			return
		}
		defer c.Close()

		tr, err := ecu.New(c, state.ecuType)
		if err != nil {
			m.output(err.Error())
			return
		}

		bin, err := tr.DumpECU(ctx, m.callback)
		if err == nil {
			m.app.SendNotification(fyne.NewNotification("", "Dump done"))
			if err := os.WriteFile(filename, bin, 0644); err == nil {
				m.output("Saved as " + filename)
			} else {
				m.output(err.Error())
			}
		} else {
			m.output(err.Error())
		}

		if err := tr.ResetECU(ctx, m.callback); err != nil {
			m.output(err.Error())
		}
	}()
}

var ter = "KE"

func (m *mainWindow) callback(v interface{}) {
	switch t := v.(type) {
	case string:
		m.output(t)
	case float64:
		if t < 0 {
			m.progressBar.Max = math.Abs(t)
			m.progressBar.SetValue(0)
			return
		}
		m.progressBar.SetValue(t)
	default:
		panic("invalid callback type")
	}
}

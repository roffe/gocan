package gui

import (
	"context"
	"log"
	"os"
	"strconv"
	"time"

	"fyne.io/fyne/v2"
	"github.com/roffe/gocan/pkg/ecu"
	sdialog "github.com/sqweek/dialog"
)

var cmd1 = []byte{
	0xBC, 0x97, 0x8D, 0x96, 0x89, 0x9E, 0xDF, 0x97,
	0x9E, 0x8C, 0xDF, 0x9E, 0xDF, 0x97, 0x8A, 0x98,
	0x9A, 0xDF, 0x9B, 0x90, 0x91, 0x98, 0xDF, 0xC3,
	0xCC,
}

func (m *mainWindow) ecuFlash() {
	if !m.checkSelections() {
		return
	}

	m.disableButtons()
	ctx, cancel := context.WithTimeout(context.Background(), 1800*time.Second)

	filename, err := sdialog.File().Filter("Bin file", "bin").Title("Load bin file").Load()
	if err != nil {
		m.output(err.Error())
		cancel()
		m.enableButtons()
		return
	}

	bin, err := os.ReadFile(filename)
	if err != nil {
		m.output(err.Error())
		cancel()
		m.enableButtons()
		return
	}

	ok := sdialog.Message("%s", "Do you want to continue?").Title("Are you sure?").YesNo()
	if !ok {
		m.enableButtons()
		cancel()
		m.output("Flash aborted by user")
		m.enableButtons()
		return
	}

	m.output("Flashing " + strconv.Itoa(len(bin)) + " bytes")
	m.progressBar.SetValue(0)
	m.progressBar.Max = float64(len(bin))
	m.progressBar.Refresh()
	state.inprogress = true

	go func() {
		defer func() {
			state.inprogress = false
		}()
		defer m.enableButtons()
		defer cancel()

		c, err := m.initCAN(ctx)
		if err != nil {
			log.Println(err)
			return
		}
		defer c.Close()

		tr, err := ecu.New(c, state.ecuType)
		if err != nil {
			m.output(err.Error())
			return
		}

		if err := tr.FlashECU(ctx, bin, m.callback); err != nil {
			m.output(err.Error())
			return
		}

		if err := tr.ResetECU(ctx, m.callback); err != nil {
			m.output(err.Error())
			return
		}

		m.app.SendNotification(fyne.NewNotification("", "Flash done"))
	}()
}

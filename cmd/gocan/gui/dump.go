package gui

import (
	"context"
	"math"
	"os"
	"time"

	"fyne.io/fyne/v2"
	"github.com/roffe/gocan/pkg/ecu"
	sdialog "github.com/sqweek/dialog"
)

func ecuDump() {
	if !checkSelections() {
		return
	}

	disableButtons()
	ctx, cancel := context.WithTimeout(context.Background(), 900*time.Second)

	filename, err := sdialog.File().Filter("bin").Title("Save binary").Save()
	if err != nil {
		output(err.Error())
		cancel()
		return
	}

	mw.progressBar.SetValue(0)

	go func() {
		defer enableButtons()
		defer cancel()

		c, err := initCAN(ctx)
		if err != nil {
			output(err.Error())
			return
		}
		defer c.Close()

		tr, err := ecu.New(c, state.ecuType)
		if err != nil {
			output(err.Error())
			return
		}

		bin, err := tr.DumpECU(ctx, callback)
		if err == nil {

			if err := os.WriteFile(filename, bin, 0644); err != nil {
				output(err.Error())
			}
		} else {
			output(err.Error())
		}

		if err := tr.ResetECU(ctx, callback); err != nil {
			output(err.Error())
		}
		mw.app.SendNotification(fyne.NewNotification("", "Dump done"))
	}()
}

func callback(v interface{}) {
	switch t := v.(type) {
	case string:
		output(t)
	case float64:
		if t < 0 {
			mw.progressBar.Max = math.Abs(t)
			mw.progressBar.SetValue(0)
			return
		}
		mw.progressBar.SetValue(t)
	default:
		panic("invalid callback type")
	}
}

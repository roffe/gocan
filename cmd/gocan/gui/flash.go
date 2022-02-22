package gui

import (
	"context"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/roffe/gocan/pkg/ecu"
	sdialog "github.com/sqweek/dialog"
)

func ecuFlash() {
	if !checkSelections() {
		return
	}

	disableButtons()
	ctx, cancel := context.WithTimeout(context.Background(), 900*time.Second)

	filename, err := sdialog.File().Filter("Select Bin", "bin").Load()
	if err != nil {
		output(err.Error())
		cancel()
		return
	}
	bin, err := os.ReadFile(filename)
	if err != nil {
		output(err.Error())
		cancel()
		return
	}

	output("Flashing " + strconv.Itoa(len(bin)) + " bytes")
	mw.progressBar.SetValue(0)
	mw.progressBar.Max = float64(len(bin))
	mw.progressBar.Refresh()
	mw.progressBar.Show()

	go func() {
		defer enableButtons()
		defer cancel()
		c, err := initCAN(ctx)
		if err != nil {
			log.Println(err)
			return
		}
		defer c.Close()
		tr, err := ecu.New(c, state.ecuType)
		if err != nil {
			output(err.Error())
			return
		}

		cb := func(n float64) {
			mw.progressBar.SetValue(n)
		}

		if err := tr.FlashECU(ctx, bin, cb); err != nil {
			output(err.Error())
			return
		}

		if err := tr.ResetECU(ctx); err != nil {
			output(err.Error())
			return
		}
		output("ECU has been reset")
	}()

}

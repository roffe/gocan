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
		enableButtons()
		return
	}

	bin, err := os.ReadFile(filename)
	if err != nil {
		output(err.Error())
		cancel()
		enableButtons()
		return
	}

	ok := sdialog.Message("%s", "Do you want to continue?").Title("Are you sure?").YesNo()
	if !ok {
		enableButtons()
		cancel()
		output("Flash aborted by user")
		return
	}

	output("Flashing " + strconv.Itoa(len(bin)) + " bytes")

	mw.progressBar.SetValue(0)
	mw.progressBar.Max = float64(len(bin))
	mw.progressBar.Refresh()

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

		if err := tr.FlashECU(ctx, bin, callback); err != nil {
			output(err.Error())
			return
		}

		if err := tr.ResetECU(ctx, callback); err != nil {
			output(err.Error())
			return
		}

		mw.app.SendNotification(fyne.NewNotification("", "Flash done"))
	}()
}

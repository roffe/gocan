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

	output("Writing " + strconv.Itoa(len(bin)) + " bytes" + filename)
	mw.progressBar.Max = float64(len(bin))
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

		if err := tr.FlashECU(ctx, bin); err != nil {
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

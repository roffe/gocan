package gui

import (
	"context"
	"log"
	"time"

	"github.com/roffe/gocan/pkg/ecu"
)

func ecuInfo() {
	if !checkSelections() {
		return
	}
	mw.progressBar.Show()
	disableButtons()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
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

		val, err := tr.Info(ctx)
		if err != nil {
			output(err.Error())
			return
		}
		mw.progressBar.Value += 50
		mw.progressBar.Refresh()
		for _, v := range val {
			output(v.String())
		}
		if err := tr.ResetECU(ctx); err != nil {
			output(err.Error())
			return
		}
		output("ECU has been reset")
	}()
}

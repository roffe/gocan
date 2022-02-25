package gui

import (
	"context"
	"time"

	"github.com/roffe/gocan/pkg/ecu"
)

func ecuInfo() {
	if !checkSelections() {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	disableButtons()

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

		val, err := tr.Info(ctx, callback)
		if err != nil {
			output(err.Error())
			return
		}

		for _, v := range val {
			output(v.String())
		}

		if err := tr.ResetECU(ctx, callback); err != nil {
			output(err.Error())
			return
		}
	}()
}

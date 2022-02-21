package gui

import (
	"context"
	"fmt"
	"strings"

	"github.com/roffe/gocan"
	"github.com/roffe/gocan/adapter/lawicel"
	"github.com/roffe/gocan/adapter/obdlink"
)

func initCAN(ctx context.Context, filters ...uint32) (*gocan.Client, error) {

	var dev gocan.Adapter
	switch strings.ToLower(state.adapter) {
	case "canusb":
		dev = lawicel.NewCanusb()
	case "sx", "obdlinksx":
		dev = obdlink.NewSX()
	default:
		return nil, fmt.Errorf("unknown adapter %q", state.adapter)
	}

	if err := dev.SetPort(state.port); err != nil {
		return nil, err
	}
	if err := dev.SetPortRate(state.portSpeed); err != nil {
		return nil, err
	}
	if err := dev.SetCANrate(state.canRate); err != nil {
		return nil, err
	}

	if err := dev.Init(ctx); err != nil {
		return nil, err
	}

	return gocan.New(ctx, dev, filters)
}

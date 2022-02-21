package gui

import (
	"context"
	"fmt"
	"strings"

	"github.com/roffe/gocan"
	"github.com/roffe/gocan/adapter/lawicel"
	"github.com/roffe/gocan/adapter/obdlink"
	"github.com/roffe/gocan/pkg/ecu"
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

	switch state.ecuType {
	case ecu.Trionic5:
		dev.SetCANfilter(0x000, 0x005, 0x006, 0x00C)
	case ecu.Trionic7:
		dev.SetCANfilter(0x220, 0x238, 0x240, 0x258, 0x266)
	case ecu.Trionic8:
		dev.SetCANfilter(0x011, 0x311, 0x7E0, 0x7E8, 0x5E8)
	}

	if err := dev.Init(ctx); err != nil {
		return nil, err
	}

	return gocan.New(ctx, dev, filters)
}

package adapter

import (
	"fmt"
	"strings"

	"github.com/roffe/gocan"
	"github.com/roffe/gocan/adapter/lawicel"
	"github.com/roffe/gocan/adapter/obdlink"
)

func New(adapter string, cfg *gocan.AdapterConfig) (dev gocan.Adapter, err error) {
	switch strings.ToLower(adapter) {
	case "canusb":
		dev, err = lawicel.NewCanusb(cfg)
	case "sx", "obdlinksx":
		dev, err = obdlink.NewSX(cfg)
	default:
		err = fmt.Errorf("unknown adapter %q", adapter)
	}
	return
}

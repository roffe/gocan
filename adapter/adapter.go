package adapter

import (
	"fmt"

	"github.com/roffe/gocan"
	"github.com/roffe/gocan/adapter/j2534"
	"github.com/roffe/gocan/adapter/lawicel"
	"github.com/roffe/gocan/adapter/obdlink"
)

type NewAdapterFunc func(*gocan.AdapterConfig) (gocan.Adapter, error)

var adapterMap = map[string]NewAdapterFunc{
	"J2534":     j2534.New,
	"CANusb":    lawicel.NewCanusb,
	"OBDLinkSX": obdlink.NewSX,
}

func New(adapterName string, cfg *gocan.AdapterConfig) (gocan.Adapter, error) {
	if adapter, found := adapterMap[adapterName]; found {
		return adapter(cfg)
	}
	return nil, fmt.Errorf("unknown adapter %q", adapterName)
}

func List() []string {
	var out []string
	for name := range adapterMap {
		out = append(out, name)
	}
	return out
}

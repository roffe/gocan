package adapter

import (
	"fmt"
	"log"

	"github.com/roffe/gocan"
	"github.com/roffe/gocan/adapter/j2534"
	"github.com/roffe/gocan/adapter/just4trionic"
	"github.com/roffe/gocan/adapter/lawicel"
	"github.com/roffe/gocan/adapter/obdlink"
)

var adapterMap = map[string]AdapterFunc{
	"J2534":        j2534.New,
	"CANusb":       lawicel.NewCanusb,
	"OBDLink SX":   obdlink.NewSX,
	"OBDLink MX":   obdlink.NewSX,
	"Just4Trionic": just4trionic.New,
}

type AdapterFunc func(*gocan.AdapterConfig) (gocan.Adapter, error)

func New(adapterName string, cfg *gocan.AdapterConfig) (gocan.Adapter, error) {
	if cfg.OutputFunc == nil {
		cfg.OutputFunc = func(s string) {
			log.Println(s)
		}
	}
	if cfg.ErrorFunc == nil {
		cfg.ErrorFunc = func(err error) {
			log.Println(err)
		}
	}
	if adapter, found := adapterMap[adapterName]; found {
		return adapter(cfg)
	}
	return nil, fmt.Errorf("unknown adapter %q", adapterName)
}

func Register(name string, adapter AdapterFunc) error {
	if _, found := adapterMap[name]; !found {
		adapterMap[name] = adapter
		return nil
	}
	return fmt.Errorf("adapter %s already registered", name)
}

func List() []string {
	var out []string
	for name := range adapterMap {
		out = append(out, name)
	}
	return out
}

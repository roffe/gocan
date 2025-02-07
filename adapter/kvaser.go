package adapter

import (
	"context"
	"log"

	"github.com/roffe/gocan"
)

/*
func init() {
	if err := Register(&AdapterInfo{
		Name:               "Kvaser",
		Description:        "Kvaser adapter",
		RequiresSerialPort: false,
		Capabilities: AdapterCapabilities{
			HSCAN: true,
			KLine: false,
			SWCAN: false,
		},
		New: NewKvaser,
	}); err != nil {
		panic(err)
	}
}
*/

type Kvaser struct {
	*BaseAdapter
}

func NewKvaser(cfg *gocan.AdapterConfig) (gocan.Adapter, error) {
	return &Kvaser{
		BaseAdapter: NewBaseAdapter(cfg),
	}, nil
}

func (a *Kvaser) SetFilter(filters []uint32) error {
	return nil
}

func (a *Kvaser) Name() string {
	return "Kvaser"
}

func (a *Kvaser) Init(ctx context.Context) error {
	if a.cfg.PrintVersion {
		// print version
		log.Println("Kvaser adapter")
	}

	return nil
}

func (a *Kvaser) Close() error {
	log.Println("Kvaser.Close()")
	a.BaseAdapter.Close()
	return nil
}

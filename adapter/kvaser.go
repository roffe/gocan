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
	cfg        *gocan.AdapterConfig
	send, recv chan gocan.CANFrame
	close      chan struct{}
}

func NewKvaser(cfg *gocan.AdapterConfig) (gocan.Adapter, error) {
	return &Kvaser{
		cfg:   cfg,
		send:  make(chan gocan.CANFrame, 10),
		recv:  make(chan gocan.CANFrame, 20),
		close: make(chan struct{}, 1),
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

func (a *Kvaser) Recv() <-chan gocan.CANFrame {
	return a.recv
}

func (a *Kvaser) Send() chan<- gocan.CANFrame {
	return a.send
}

func (a *Kvaser) Close() error {
	log.Println("Kvaser.Close()")
	return nil
}

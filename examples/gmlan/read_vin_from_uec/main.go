package main

import (
	"context"
	"log"

	"github.com/roffe/gocan"
	"github.com/roffe/gocan/pkg/gmlan"
)

func main() {
	dev, err := gocan.NewAdapter(
		"J2534",
		&gocan.AdapterConfig{
			Port:         `C:\Program Files (x86)\Drew Technologies, Inc\J2534\MongoosePro GM II\monpa432.dll`,
			PortBaudrate: 0,
			CANRate:      33.3,
			CANFilter:    []uint32{0x64F},
		},
	)
	if err != nil {
		log.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c, err := gocan.NewWithAdapter(ctx, dev)
	if err != nil {
		log.Fatal(err)
	}
	defer c.Close()

	gm := gmlan.New(c, 0x24F, 0x64F)

	gm.TesterPresentNoResponseAllowed()

	if err := gm.InitiateDiagnosticOperation(ctx, 0x02); err != nil {
		log.Fatal(err)
	}
	defer func() {
		if err := gm.ReturnToNormalMode(ctx); err != nil {
			log.Println(err)
		}
	}()

	if err := gm.DisableNormalCommunication(ctx); err != nil {
		log.Fatal(err)
	}

	vin, err := gm.ReadDataByIdentifierString(ctx, 0x90)
	if err != nil {
		log.Fatal(err)
	}
	log.Println("VIN:", vin)
}

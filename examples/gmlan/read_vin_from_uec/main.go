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
			CANRate:      500,
			CANFilter:    []uint32{0x64F},
		},
	)
	if err != nil {
		log.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cl, err := gocan.NewWithOpts(ctx, dev)
	if err != nil {
		log.Fatal(err)
	}

	go func() {
		defer cl.Close()
		//gm := gmlan.New(cl, 0x24F, 0x64F)
		gm := gmlan.New(cl, 0x7E0, 0x7E8)

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
	}()

	if err := cl.Wait(ctx); err != nil {
		log.Fatal(err)
	}
}

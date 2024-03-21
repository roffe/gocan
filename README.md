# goCAN

A Go linux/windows/osx CAN stack running in userland with low level adapter drivers written from the ground up
J2534 support in Windows / Linux

## USB Serial

* OBDLinx SX/EX/MX/MX+: https://www.obdlink.com/
* STN1130
* STN1170
* STN2120
* [CANUSB](https://lawicel-shop.se/elektronik/kommunikation/can/lawicel-canusb-adapter-1m-usb-cable/) adapter running in VCP mode using [Lawicel ascii api](http://www.can232.com/docs/canusb_manual.pdf)
* CANable Nano and Pro running [slcan](https://github.com/normaldotcom/canable-fw)
* [YACA](https://github.com/roffe/yaca)

## J2534

### Windows
* Drewtech Mongoose GM PRO II: https://www.drewtech.com/
* Scanmatik 2 PRO: https://scanmatik.pro/
* Tech2 passthru ( limited support )
* GM MDI

### Linux
* Tactrix Openport 2.0: https://github.com/dschultzca/j2534
* Machina: https://github.com/rnd-ash/Macchina-J2534
* WQCAN: https://github.com/witoldo7/STM32CAN

## Other

* SocketCAN

## Showcase

[Saab CAN flasher](https://github.com/roffe/gocanflasher)
[txlogger](https://github.com/roffe/txlogger)

## Example

```go
package main

import (
	"context"
	"log"

	"github.com/roffe/gocan"
	"github.com/roffe/gocan/adapter"
	"github.com/roffe/gocan/pkg/gmlan"
)

func main() {
	dev, err := adapter.New(
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
	c, err := gocan.New(ctx, dev)
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
```
    > $env:CGO_ENABLED=1; $env:GOARCH=386; go run .\examples\gmlan\read_vin_from_uec\main.go 
    VIN: YS3FB45F231027975

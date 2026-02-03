//go:build ftdi

package gocan

import (
	"log"

	ftdi "github.com/roffe/gocan/pkg/ftdi"
)

func init() {
	if err := ftdi.Init(); err != nil {
		log.Println("ftd2xx driver not loaded:", err)
		return
	}

	devs, err := ftdi.GetDeviceList()
	if err != nil {
		log.Printf("ftd2xx failed to get device list: %v", err)
		return
	}
	for _, dev := range devs {
		//log.Println("Found FTDI device:", dev.Description, "S/N:", dev.SerialNumber)
		switch dev.Description {
		case OBDLinkSX, OBDLinkEX, STN1170, STN2120:
			if err := RegisterAdapter(&AdapterInfo{
				Name:               "d2xx " + dev.Description,
				Description:        "ftdi d2xx " + dev.Description,
				RequiresSerialPort: false,
				Capabilities: AdapterCapabilities{
					HSCAN: true,
					KLine: true,
					SWCAN: false,
				},
				New: NewScantoolFTDI("d2xx "+dev.Description, dev.Index, dev.SerialNumber),
			}); err != nil {
				panic(err)
			}
		case "CANUSB":
			name := "d2xx CANUSB " + dev.SerialNumber
			if err := RegisterAdapter(&AdapterInfo{
				Name:               name,
				Description:        "Lawicell CANUSB over d2xx",
				RequiresSerialPort: false,
				Capabilities: AdapterCapabilities{
					HSCAN: true,
					KLine: false,
					SWCAN: true,
				},
				New: NewCanusbFTDI(name, dev.Index, dev.SerialNumber),
			}); err != nil {
				panic(err)
			}
		}
	}

}

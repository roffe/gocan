package lawicel

import (
	"context"
	"fmt"
	"log"

	"go.bug.st/serial"
)

type Canusb struct {
	port             serial.Port
	portName         string
	portRate         int
	canRate          string
	canCode, canMask string
}

func (cu *Canusb) Init() error {
	mode := &serial.Mode{
		BaudRate: cu.portRate,
		Parity:   serial.NoParity,
		DataBits: 8,
		StopBits: serial.OneStopBit,
	}
	p, err := serial.Open(cu.portName, mode)
	if err != nil {
		return fmt.Errorf("failed to open com port %q : %v", cu.portName, err)
	}
	cu.port = p

	var cmds = []string{
		"\r\r",     // Empty buffer
		"V",        // Get Version number of both CANUSB hardware and software
		"N",        // Get Serial number of the CANUSB
		"Z0",       // Sets Time Stamp OFF for received frames
		cu.canRate, // Setup CAN bit-rates
		cu.canCode,
		cu.canMask,
		"O", // Open the CAN channel
	}
	log.Println(cmds)

	return nil
}

func (cu *Canusb) SetPort(port string) error {
	cu.portName = port
	return nil
}

func (cu *Canusb) SetPortRate(rate int) error {
	cu.portRate = rate
	return nil
}

func (cu *Canusb) SetCANrate(rate float64) error {
	switch rate {
	case 10:
		cu.canRate = "S0"
	case 20:
		cu.canRate = "S1"
	case 33:
		//cu.canRate = "s8b2f"
		cu.canRate = "s0e1c"
	case 47.619:
		// BTR0 0xCB, BTR1 0x9A
		cu.canRate = "scb9a"
	case 50:
		cu.canRate = "S2"
	case 100:
		cu.canRate = "S3"
	case 125:
		cu.canRate = "S4"
	case 250:
		cu.canRate = "S5"
	case 500:
		cu.canRate = "S6"
	case 615:
		cu.canRate = "s4037"
	case 800:
		cu.canRate = "S7"
	case 1000:
		cu.canRate = "S8"
	default:
		return fmt.Errorf("unknown rate: %f", rate)

	}
	return nil
}

func (cu *Canusb) SetCANfilter(ids ...uint32) {
	cu.canCode, cu.canMask = calcAcceptanceFilters(ids...)
}

func (cu *Canusb) Read(ctx context.Context) ([]byte, error) {
	return nil, nil

}
func (cu *Canusb) Write(ctx context.Context, data []byte) error {
	return nil
}

func calcAcceptanceFilters(idList ...uint32) (string, string) {
	var code uint32 = ^uint32(0)
	var mask uint32 = 0
	if len(idList) == 0 {
		code = 0
		mask = ^uint32(0)
	} else {
		for _, canID := range idList {
			if canID == 0x00 {
				log.Println("Found illegal id: ", canID)
				code = 0
				mask = 0
				break
			}
			code &= (canID & 0x7FF) << 5
			mask |= (canID & 0x7FF) << 5
		}
	}
	code |= code << 16
	mask |= mask << 16

	return fmt.Sprintf("M%08X", code), fmt.Sprintf("m%08X", mask)
}

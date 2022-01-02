package canusb

import (
	"fmt"

	"go.bug.st/serial"
)

type Opts func(c *Canusb) error

// Canusb runs
// clock freq 16
// saab i-bus BTR0=CB and BTR1=9A
// saab p-bus 500kbit

func OptComPort(port string, baudrate int) Opts {
	return func(c *Canusb) error {
		portInfo(port)
		mode := &serial.Mode{
			BaudRate: baudrate,
			Parity:   serial.NoParity,
			DataBits: 8,
			StopBits: serial.OneStopBit,
		}
		p, err := serial.Open(port, mode)
		if err != nil {
			return fmt.Errorf("failed to open com port %q : %v", port, err)
		}
		c.port = p
		return nil
	}
}

func OptRate(kbit float64) Opts {
	return func(c *Canusb) error {
		switch kbit {
		case 10:
			c.canrate = "S0"
		case 20:
			c.canrate = "S1"
		case 47.619:
			// BTR0 0xCB, BTR1 0x9A
			c.canrate = "scb9a"
		case 50:
			c.canrate = "S2"
		case 100:
			c.canrate = "S3"
		case 125:
			c.canrate = "S4"
		case 250:
			c.canrate = "S5"
		case 500:
			c.canrate = "S6"
		case 615:
			c.canrate = "s4037"
		case 800:
			c.canrate = "S7"
		case 1000:
			c.canrate = "S8"
		default:
			return fmt.Errorf("unknown rate: %f", kbit)

		}
		return nil
	}
}

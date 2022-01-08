package canusb

import (
	"errors"
	"fmt"
	"log"
	"runtime"
	"strings"

	"go.bug.st/serial"
	"go.bug.st/serial/enumerator"
)

type Opts func(c *Canusb) error

// Canusb runs
// clock freq 16Mhz
// saab i-bus BTR0=CB and BTR1=9A
// saab p-bus 500kbit

func OptComPort(port string, baudrate int) Opts {
	return func(c *Canusb) error {
		portSelected, err := portInfo(port)
		if err != nil {
			return err
		}
		mode := &serial.Mode{
			BaudRate: baudrate,
			Parity:   serial.NoParity,
			DataBits: 8,
			StopBits: serial.OneStopBit,
		}
		p, err := serial.Open(portSelected, mode)
		if err != nil {
			return fmt.Errorf("failed to open com port %q : %v", port, err)
		}
		c.port = p
		return nil
	}
}

func OptCabLogging(enabled bool) Opts {
	return func(c *Canusb) error {
		c.logging = enabled
		return nil
	}
}

func portInfo(portName string) (string, error) {
	if runtime.GOOS == "windows" {
		portName = strings.ToUpper(portName)
	}
	ports, err := enumerator.GetDetailedPortsList()
	if err != nil {
		return "", err
	}
	if len(ports) == 0 {
		return "", errors.New("no serial ports found")
	}
	if portName == "*" {
		log.Println("discovered com ports:")
	}

	for _, port := range ports {
		if port.Name == portName || portName == "*" {
			log.Printf("port: %s\n", port.Name)
			if port.IsUSB {
				log.Printf("   USB ID      %s:%s\n", port.VID, port.PID)
				log.Printf("   USB serial  %s\n", port.SerialNumber)
			}
			if portName == "*" {
				continue
			}
			return portName, nil
		}
	}
	return "", errors.New("no device selected")
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

//go:build ftdi

package canusb

import (
	"fmt"
	"time"

	gocan "github.com/roffe/gocan/v2"
	"github.com/roffe/gocan/v2/pkg/ftdi"
)

// Registers Lawicel CANUSB devices attached via the FTDI D2XX driver as
// "d2xx CANUSB <serial>". Opt-in with the "ftdi" build tag (needs the D2XX
// driver on Windows / libftdi on Linux).
func init() {
	if err := ftdi.Init(); err != nil {
		return
	}
	devs, err := ftdi.GetDeviceList()
	if err != nil {
		return
	}
	for _, dev := range devs {
		if dev.Description != "CANUSB" {
			continue
		}
		name := "d2xx CANUSB " + dev.SerialNumber
		index, serialNo := dev.Index, dev.SerialNumber
		gocan.Register(gocan.AdapterInfo{
			Name:         name,
			Description:  "Lawicell CANUSB over d2xx",
			Capabilities: gocan.Capabilities{HSCAN: true, SWCAN: true},
			New: func(cfg gocan.Config) (gocan.Adapter, error) {
				a, err := New(cfg)
				if err != nil {
					return nil, err
				}
				cu := a.(*CANUSB)
				cu.openPort = func() (serialPort, error) {
					return openD2XX(index, serialNo)
				}
				return cu, nil
			},
		})
	}
}

// d2xxPort adapts an FTDI D2XX device to the serialPort interface.
type d2xxPort struct {
	*ftdi.Device
}

func (d d2xxPort) SetReadTimeout(t time.Duration) error {
	ms := int(t.Milliseconds())
	if ms < 1 {
		ms = 1
	}
	return d.SetTimeout(ms, ms)
}

func (d d2xxPort) ResetInputBuffer() error  { return d.Purge(ftdi.FT_PURGE_RX) }
func (d d2xxPort) ResetOutputBuffer() error { return d.Purge(ftdi.FT_PURGE_TX) }

func openD2XX(index uint64, serialNo string) (serialPort, error) {
	p, err := ftdi.Open(ftdi.DeviceInfo{Index: index, SerialNumber: serialNo}, 0x6001)
	if err != nil {
		return nil, fmt.Errorf("failed to open ftdi device: %w", err)
	}
	if err := p.SetLineProperty(ftdi.LineProperties{Bits: 8, StopBits: 0, Parity: ftdi.NONE}); err != nil {
		p.Close()
		return nil, err
	}
	if err := p.SetBaudRate(3000000); err != nil {
		p.Close()
		return nil, err
	}
	if err := p.SetLatency(1); err != nil {
		p.Close()
		return nil, err
	}
	if err := p.SetTimeout(4, 4); err != nil {
		p.Close()
		return nil, err
	}
	return d2xxPort{p}, nil
}

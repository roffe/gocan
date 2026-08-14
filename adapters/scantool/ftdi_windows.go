//go:build ftdi

package scantool

import (
	"fmt"
	"time"

	gocan "github.com/roffe/gocan/v2"
	"github.com/roffe/gocan/v2/pkg/ftdi"
)

// Registers STN adapters attached via the FTDI D2XX driver as
// "d2xx <model>". Opt-in with the "ftdi" build tag (needs the D2XX driver
// on Windows / libftdi on Linux).
func init() {
	gocan.RegisterScanner(scanD2XXDevices)
}

func scanD2XXDevices() []gocan.AdapterInfo {
	if err := ftdi.Init(); err != nil {
		return nil
	}
	devs, err := ftdi.GetDeviceList()
	if err != nil {
		return nil
	}
	var out []gocan.AdapterInfo
	for _, dev := range devs {
		switch dev.Description {
		case OBDLinkSX, OBDLinkEX, STN1170, STN2120:
		default:
			continue
		}
		baseName := dev.Description
		name := "d2xx " + baseName
		index, serialNo := dev.Index, dev.SerialNumber
		out = append(out, gocan.AdapterInfo{
			Name:         name,
			Description:  "ftdi d2xx " + baseName,
			Capabilities: gocan.Capabilities{HSCAN: true, KLine: true},
			New: func(cfg gocan.Config) (gocan.Adapter, error) {
				// the base model name drives the STP protocol/bit-rate table
				a, err := New(baseName, cfg)
				if err != nil {
					return nil, err
				}
				st := a.(*Scantool)
				st.name = name
				st.openPort = func() (port, error) {
					return openD2XX(index, serialNo)
				}
				return st, nil
			},
		})
	}
	return out
}

// d2xxPort adapts an FTDI D2XX device to the port interface.
type d2xxPort struct {
	*ftdi.Device
}

func (d d2xxPort) SetBaud(baud int) error { return d.SetBaudRate(uint(baud)) }

func (d d2xxPort) SetReadTimeout(t time.Duration) error {
	ms := int(t.Milliseconds())
	if ms < 1 {
		ms = 1
	}
	return d.SetTimeout(ms, ms)
}

func (d d2xxPort) ResetInputBuffer() error  { return d.Purge(ftdi.FT_PURGE_RX) }
func (d d2xxPort) ResetOutputBuffer() error { return d.Purge(ftdi.FT_PURGE_TX) }

func openD2XX(index uint64, serialNo string) (port, error) {
	p, err := ftdi.Open(ftdi.DeviceInfo{Index: index, SerialNumber: serialNo}, 0x6015)
	if err != nil {
		return nil, fmt.Errorf("failed to open ftdi device: %w", err)
	}
	if err := p.SetLineProperty(ftdi.LineProperties{Bits: 8, StopBits: 0, Parity: ftdi.NONE}); err != nil {
		p.Close()
		return nil, err
	}
	if err := p.SetLatency(1); err != nil {
		p.Close()
		return nil, err
	}
	if err := p.SetTimeout(10, 10); err != nil {
		p.Close()
		return nil, err
	}
	return d2xxPort{p}, nil
}

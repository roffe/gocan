//go:build ble
// +build ble

package adapter

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/roffe/gocan"
	"github.com/roffe/gocan/dvi"
	"tinygo.org/x/bluetooth"
)

var adapter = bluetooth.DefaultAdapter

func init() {
	if err := Register(&AdapterInfo{
		Name:               "OBDX Pro GT BLE",
		Description:        "OBDX Pro GT BLE",
		RequiresSerialPort: false,
		Capabilities: AdapterCapabilities{
			HSCAN: true,
			KLine: true,
			SWCAN: true,
		},
		New: NewOBDXPro,
	}); err != nil {
		panic(err)
	}
	if err := adapter.Enable(); err != nil {
		panic(err)
	}
}

type OBDXProBLE struct {
	device bluetooth.Device

	filters []*dvi.Command

	//adapter    *bluetooth.Adapter
	cfg        *gocan.AdapterConfig
	send, recv chan gocan.CANFrame
	close      chan struct{}
	closeOnce  sync.Once

	recvBuff *bytes.Buffer

	tx, rx bluetooth.DeviceCharacteristic
}

func NewOBDXProBLE(cfg *gocan.AdapterConfig) (gocan.Adapter, error) {
	return &OBDXPro{
		//adapter:  btadapter,
		cfg:      cfg,
		send:     make(chan gocan.CANFrame, 10),
		recv:     make(chan gocan.CANFrame, 10),
		close:    make(chan struct{}),
		recvBuff: bytes.NewBuffer(nil),
	}, nil
}

func (a *OBDXPro) SetFilter(filters []uint32) error {
	for i, id := range filters {
		filterCMD := dvi.New(0x34, []byte{0x00, byte(i), dvi.FRAME_TYPE_11BIT, dvi.FILTER_TYPE_PASS, dvi.FILTER_STATUS_ON, byte(id >> 24), byte(id >> 16), byte(id >> 8), byte(id), 0x00, 0x00, 0x07, 0xFF, 0x00, 0x00, 0x00, 0x00})
		a.filters = append(a.filters, filterCMD)
	}
	return nil
}

func (a *OBDXPro) Name() string {
	return "OBDX Pro"
}

func (a *OBDXPro) Init(ctx context.Context) error {

	if err := a.SetFilter(a.cfg.CANFilter); err != nil {
		return err
	}

	var err error
	a.device, err = a.connectDevice()
	if err != nil {
		return err
	}

	svc, err := a.getUARTService()
	if err != nil {
		return err
	}

	if err := a.getRxTx(svc); err != nil {
		return err
	}

	gogo := false
	parser := NewCommandParser(a.handleCommand)
	err = a.rx.EnableNotifications(func(buf []byte) {
		if gogo {
			parser.AddData(buf)
		}
	})
	if err != nil {
		return err
	}

	a.tx.Write([]byte("ATAR\r"))
	time.Sleep(20 * time.Millisecond)

	a.tx.Write([]byte("DXDP1\r"))
	time.Sleep(50 * time.Millisecond)
	gogo = true

	var initCommands = []*dvi.Command{
		dvi.New(0x31, []byte{0x01, 0x02}),       // set HS CAN
		dvi.New(0x34, []byte{0x15, 0x06}),       // set CAN speed to 500kbit
		dvi.New(0x34, []byte{0x0F, 0x00}),       // disable automatic formating for writing network frames
		dvi.New(0x34, []byte{0x0B, 0x00}),       // disable Automatic Frame Processing for Received Network Messages
		dvi.New(0x34, []byte{0x0E, 0x00}),       // disable padding
		dvi.New(0x34, []byte{0x10, 0x00, 0xFA}), // delay between writing frames
		dvi.New(0x24, []byte{0x01, 0x00}),       // disable network write responses status

	}

	for _, cmd := range initCommands {
		if _, err := a.tx.Write(cmd.Bytes()); err != nil {
			a.cfg.OnError(fmt.Errorf("failed to send init command: %v", err))
		}
		time.Sleep(15 * time.Millisecond)
	}

	for _, f := range a.filters {
		if _, err := a.tx.Write(f.Bytes()); err != nil {
			a.cfg.OnError(fmt.Errorf("failed to set filter: %v", err))
		}
		time.Sleep(10 * time.Millisecond)
	}

	// enable networking
	if _, err := a.tx.Write(dvi.New(0x31, []byte{0x02, 0x01}).Bytes()); err != nil {
		return fmt.Errorf("failed to enable networking: %v", err)
	}

	go func() {
		for {
			select {
			case <-a.close:
				return
			case frame := <-a.send:
				id := frame.Identifier()
				sendCmd := dvi.New(dvi.CMD_SEND_TO_NETWORK_NORMAL, append([]byte{byte(id >> 24), byte(id >> 16), byte(id >> 8), byte(id)}, frame.Data()...))
				if a.cfg.Debug {
					log.Println("dvi out:", sendCmd.String())
				}
				if _, err := a.tx.Write(sendCmd.Bytes()); err != nil {
					log.Printf("Failed to send DVI: %v", err)
				}

			}
		}
	}()

	return nil
}

func (a *OBDXPro) Recv() <-chan gocan.CANFrame {
	return a.recv
}

func (a *OBDXPro) Send() chan<- gocan.CANFrame {
	return a.send
}

func (a *OBDXPro) Close() (err error) {
	a.closeOnce.Do(func() {
		close(a.close)
		time.Sleep(80 * time.Millisecond)

		cmds := []*dvi.Command{
			dvi.New(0x31, []byte{0x02, 0x00}), // disable networking
			dvi.New(0x25, []byte{}),           // reset
		}

		for _, cmd := range cmds {
			if _, err = a.tx.Write(cmd.Bytes()); err != nil {
				log.Printf("Failed to send command: %v", err)
			}
			time.Sleep(20 * time.Millisecond)
		}
		err = a.device.Disconnect()

		a.cfg.OnMessage("Disconnected from OBDX Pro")
		time.Sleep(100 * time.Millisecond)
	})
	return err
}

func (a *OBDXPro) connectDevice() (bluetooth.Device, error) {
	a.cfg.OnMessage("Scanning for adapter")
	ch := make(chan bluetooth.ScanResult, 1)
	start := time.Now()
	if err := adapter.Scan(func(adapter *bluetooth.Adapter, device bluetooth.ScanResult) {
		if time.Since(start) > 10*time.Second {
			adapter.StopScan()
		}
		//println("found device:", device.Address.String(), device.RSSI, device.LocalName())
		//if a.cfg.Debug {
		//	a.cfg.OnMessage(fmt.Sprintf("saw device: %s, %d, %s", device.Address.String(), device.RSSI, device.LocalName()))
		//}
		if strings.HasPrefix(device.LocalName(), "OBDX Pro") {
			a.cfg.OnMessage(fmt.Sprintf("Found device: %s 📶 %d", device.LocalName(), device.RSSI))
			adapter.StopScan()
			ch <- device
		}
	}); err != nil {
		return bluetooth.Device{}, err
	}

	select {
	case d := <-ch:
		device, err := adapter.Connect(d.Address, bluetooth.ConnectionParams{})
		if err != nil {
			return bluetooth.Device{}, fmt.Errorf("failed to connect to %s: %w", d.LocalName(), err)
		}
		a.cfg.OnMessage(fmt.Sprintf("Connecting to %s %s", d.LocalName(), d.Address.String()))
		return device, nil
	default:
		return bluetooth.Device{}, errors.New("did not find any suitable device")
	}
}

func (a *OBDXPro) getUARTService() (bluetooth.DeviceService, error) {
	svcs, err := a.device.DiscoverServices([]bluetooth.UUID{bluetooth.ServiceUUIDNordicUART})
	if err != nil {
		return bluetooth.DeviceService{}, fmt.Errorf("failed to discover services: %w", err)
	}
	if len(svcs) == 0 {
		return bluetooth.DeviceService{}, errors.New("no UART service found")
	}
	return svcs[0], nil
}

func (a *OBDXPro) getRxTx(srvc bluetooth.DeviceService) error {
	chars, err := srvc.DiscoverCharacteristics([]bluetooth.UUID{bluetooth.CharacteristicUUIDUARTTX, bluetooth.CharacteristicUUIDUARTRX})
	if err != nil {
		return fmt.Errorf("failed to discover BLE characteristics: %w", err)
	}
	cnt := 0
	for _, char := range chars {
		if char.String() == bluetooth.CharacteristicUUIDUARTTX.String() {
			a.rx = char
			cnt++
			continue
		}
		if char.String() == bluetooth.CharacteristicUUIDUARTRX.String() {
			a.tx = char
			cnt++
			continue
		}
	}
	if cnt != 2 {
		return errors.New("failed to find tx/rx characteristics")
	}
	return nil
}

// Example handler function
func (a *OBDXPro) handleCommand(cmd *dvi.Command) {
	if a.cfg.Debug {
		log.Println("dvi in:", cmd.String())
	}
	switch cmd.Command() {
	case 0x08:
		id := binary.BigEndian.Uint32(cmd.Data()[:4])
		frame := gocan.NewFrame(
			id,
			cmd.Data()[4:],
			gocan.Incoming,
		)
		select {
		case a.recv <- frame:
		default:
			log.Printf("Dropped frame: %s", frame.String())
		}
		return
	}
}

package gocan

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"go.bug.st/serial"
)

func init() {
	if err := RegisterAdapter(&AdapterInfo{
		Name:               OBDLinkSX,
		Description:        "ScanTool.net " + OBDLinkSX,
		RequiresSerialPort: true,
		Capabilities: AdapterCapabilities{
			HSCAN: true,
			KLine: false,
			SWCAN: false,
		},
		New: NewSTNVCP(OBDLinkSX),
	}); err != nil {
		panic(err)
	}
	if err := RegisterAdapter(&AdapterInfo{
		Name:               OBDLinkEX,
		Description:        "ScanTool.net " + OBDLinkEX,
		RequiresSerialPort: true,
		Capabilities: AdapterCapabilities{
			HSCAN: true,
			KLine: false,
			SWCAN: false,
		},
		New: NewSTNVCP(OBDLinkEX),
	}); err != nil {
		panic(err)
	}
	if err := RegisterAdapter(&AdapterInfo{
		Name:               STN1170,
		Description:        "ScanTool.net STN1170 based adapter",
		RequiresSerialPort: true,
		Capabilities: AdapterCapabilities{
			HSCAN: true,
			KLine: true,
			SWCAN: true,
		},
		New: NewSTNVCP(STN1170),
	}); err != nil {
		panic(err)
	}
	if err := RegisterAdapter(&AdapterInfo{
		Name:               STN2120,
		Description:        "ScanTool.net STN2120 based adapter",
		RequiresSerialPort: true,
		Capabilities: AdapterCapabilities{
			HSCAN: true,
			KLine: true,
			SWCAN: true,
		},
		New: NewSTNVCP(STN2120),
	}); err != nil {
		panic(err)
	}
}

type STNVCP struct {
	BaseAdapter

	baseName     string
	canrateCMD   string
	protocolCMD  string
	filter, mask string
	sendSem      chan struct{}

	port serial.Port
}

func NewSTNVCP(name string) func(cfg *AdapterConfig) (Adapter, error) {
	return func(cfg *AdapterConfig) (Adapter, error) {
		stn := &STNVCP{
			BaseAdapter: NewBaseAdapter(name, cfg),
			sendSem:     make(chan struct{}, 1),
			baseName:    name,
		}
		var err error
		stn.protocolCMD, stn.canrateCMD, err = scantoolCalculateCANrate(stn.baseName, cfg.CANRate)
		if err != nil {
			return nil, err
		}
		stn.filter, stn.mask = scantoolCANfilter(cfg.CANFilter)
		return stn, nil
	}
}

func (stn *STNVCP) SetFilter(filters []uint32) error {
	stn.filter, stn.mask = scantoolCANfilter(stn.cfg.CANFilter)
	return scantoolSetFilter(&stn.BaseAdapter, stn.filter, stn.mask)
}

func (stn *STNVCP) Open(ctx context.Context) error {
	mode := &serial.Mode{
		BaudRate: stn.cfg.PortBaudrate,
		Parity:   serial.NoParity,
		DataBits: 8,
		StopBits: serial.OneStopBit,
	}
	var err error
	stn.port, err = serial.Open(stn.cfg.Port, mode)
	if err != nil {
		return fmt.Errorf("failed to open com port %q : %v", stn.cfg.Port, err)
	}

	if err := stn.port.SetReadTimeout(10 * time.Millisecond); err != nil {
		stn.port.Close()
		return err
	}

	to := uint(2000000)
	found := false

	resetInputBuffer := func() error {
		return stn.port.ResetInputBuffer()
	}
	speedSetter := func(baud int) error {
		return stn.port.SetMode(&serial.Mode{
			BaudRate: baud,
			Parity:   serial.NoParity,
			DataBits: 8,
			StopBits: serial.OneStopBit,
		})
	}

	for _, from := range scantoolBaudrates {
		log.Println("trying to change baudrate from", from, "to", to, "bps")
		if err := scantoolTrySpeed(stn.port, from, to, speedSetter, resetInputBuffer, stn.cfg.OnMessage); err == nil {
			found = true
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if !found {
		stn.port.Close()
		return errors.New("failed to switch adapter baudrate")
	}

	scantoolInit(stn.cfg.Debug, stn.port, stn.protocolCMD, stn.canrateCMD, stn.filter, stn.mask, stn.cfg.OnMessage)
	if err := stn.port.ResetInputBuffer(); err != nil {
		stn.port.Close()
		return err
	}

	go stn.recvManager(ctx)
	go scantoolSendManager(ctx, stn.cfg.Debug, stn.port, stn.sendChan, stn.sendSem, stn.closeChan, stn.SetError, stn.cfg.OnMessage)

	return nil
}

func (stn *STNVCP) Close() error {
	stn.BaseAdapter.Close()
	time.Sleep(50 * time.Millisecond)
	stn.port.Write([]byte("ATZ\r"))
	time.Sleep(100 * time.Millisecond)
	stn.port.ResetInputBuffer()
	stn.port.ResetOutputBuffer()
	return stn.port.Close()
}

func (stn *STNVCP) recvManager(ctx context.Context) {
	buff := bytes.NewBuffer(nil)
	readBuffer := make([]byte, 64)
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		n, err := stn.port.Read(readBuffer)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			stn.SetError(fmt.Errorf("failed to read: %w", err))
			return
		}
		if n == 0 {
			continue
		}
		for _, b := range readBuffer[:n] {
			//select {
			//case <-ctx.Done():
			//	return
			//default:
			//}
			if b == '>' {
				select {
				case <-stn.sendSem:
				default:
				}
				continue
			}

			if b == 0x0D {
				if buff.Len() == 0 {
					continue
				}
				if stn.cfg.Debug {
					stn.cfg.OnMessage("<i> " + buff.String())
				}
				switch buff.String() {
				case "CAN ERROR":
					stn.cfg.OnMessage("CAN ERROR")
					buff.Reset()
				case "STOPPED":
					stn.cfg.OnMessage("STOPPED")
					buff.Reset()
				case "?":
					stn.cfg.OnMessage("UNKNOWN COMMAND")
					buff.Reset()
				case "NO DATA", "OK":
					buff.Reset()
				default:
					f, err := scantoolDecodeFrame(buff.Bytes())
					if err != nil {
						stn.cfg.OnMessage(fmt.Sprintf("failed to decode frame: %s %v", buff.String(), err))
						buff.Reset()
						continue
					}
					select {
					case stn.recvChan <- f:
					default:
						stn.SetError(ErrDroppedFrame)
					}
					buff.Reset()
				}
				continue
			}
			buff.WriteByte(b)
		}
	}
}

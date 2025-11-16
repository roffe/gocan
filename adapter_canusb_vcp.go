package gocan

import (
	"bytes"
	"context"
	"fmt"
	"time"

	"go.bug.st/serial"
)

func init() {
	if err := RegisterAdapter(&AdapterInfo{
		Name:               "CANUSB VCP",
		Description:        "Lawicell CANUSB",
		RequiresSerialPort: true,
		Capabilities: AdapterCapabilities{
			HSCAN: true,
			KLine: false,
			SWCAN: true,
		},
		New: NewCanusbVCP,
	}); err != nil {
		panic(err)
	}
}

type CanusbVCP struct {
	BaseAdapter
	port         serial.Port
	canRate      string
	filter, mask string
	buff         *bytes.Buffer
	sendSem      chan struct{}
}

func NewCanusbVCP(cfg *AdapterConfig) (Adapter, error) {
	cu := &CanusbVCP{
		BaseAdapter: NewBaseAdapter("CANUSB VCP", cfg),
		buff:        bytes.NewBuffer(nil),
		sendSem:     make(chan struct{}, 1),
	}
	rate, err := canusbCANrate(cfg.CANRate)
	if err != nil {
		return nil, err
	}
	cu.canRate = rate
	cu.filter, cu.mask = canusbAcceptanceFilters(cfg.CANFilter)
	return cu, nil
}

func (cu *CanusbVCP) SetFilter(filters []uint32) error {
	return canusbSetFilter(&cu.BaseAdapter, filters)
}

func (cu *CanusbVCP) Open(ctx context.Context) error {
	mode := &serial.Mode{
		BaudRate: cu.cfg.PortBaudrate,
		Parity:   serial.NoParity,
		DataBits: 8,
		StopBits: serial.OneStopBit,
	}
	p, err := serial.Open(cu.cfg.Port, mode)
	if err != nil {
		return fmt.Errorf("failed to open com port %q : %v", cu.cfg.Port, err)
	}
	p.SetReadTimeout(4 * time.Millisecond)
	cu.port = p

	if err := canusbInit(cu.port, cu.canRate, cu.filter, cu.mask); err != nil {
		cu.port.Close()
		return err
	}

	p.ResetInputBuffer()

	go cu.recvManager(ctx)
	go canusbSendManager(ctx, &cu.BaseAdapter, cu.sendSem, cu.port)

	// Open the CAN channel
	cu.sendChan <- &CANFrame{
		Identifier: SystemMsg,
		Data:       []byte{'O'},
		FrameType:  Outgoing,
	}

	return nil
}

func (cu *CanusbVCP) Close() error {
	cu.BaseAdapter.Close()
	if cu.port != nil {
		if err := canusbClose(cu.port); err != nil {
			return fmt.Errorf("canusb vcp close error: %w", err)
		}
		cu.port.ResetInputBuffer()
		cu.port.ResetOutputBuffer()
		if err := cu.port.Close(); err != nil {
			return fmt.Errorf("failed to close com port: %w", err)
		}
		cu.port = nil
	}
	return nil
}

func (cu *CanusbVCP) recvManager(ctx context.Context) {
	parseFn := canusbCreateParser(cu.buff, &cu.BaseAdapter, cu.sendSem)

	readBuffer := make([]byte, 64)
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		n, err := cu.port.Read(readBuffer)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			cu.Fatal(fmt.Errorf("failed to read com port: %w", err))
			return
		}
		select {
		case <-cu.closeChan:
			return
		default:
			if n == 0 {
				continue
			}
			parseFn(readBuffer[:n])
		}

	}
}

//go:build ftdi

package gocan

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"time"

	ftdi "github.com/roffe/goftdi"
)

type CanusbFTDI struct {
	BaseAdapter
	port         *ftdi.Device
	canRate      string
	filter, mask string
	buff         *bytes.Buffer
	sendSem      chan struct{}
	devIndex     uint64
}

func NewCanusbFTDI(name string, index uint64) func(cfg *AdapterConfig) (Adapter, error) {
	return func(cfg *AdapterConfig) (Adapter, error) {
		cu := &CanusbFTDI{
			BaseAdapter: NewBaseAdapter(name, cfg),
			buff:        bytes.NewBuffer(nil),
			//sendMutex: make(chan token, 1),
			sendSem:  make(chan struct{}, 1),
			devIndex: index,
		}
		if err := cu.setCANrate(cfg.CANRate); err != nil {
			return nil, err
		}
		cu.filter, cu.mask = cu.calcAcceptanceFilters(cfg.CANFilter)
		return cu, nil
	}
}

func (cu *CanusbFTDI) SetFilter(filters []uint32) error {
	filter, mask := cu.calcAcceptanceFilters(filters)
	cu.Send() <- NewFrame(SystemMsg, []byte{'C'}, Outgoing)
	cu.Send() <- NewFrame(SystemMsg, []byte(filter), Outgoing)
	cu.Send() <- NewFrame(SystemMsg, []byte(mask), Outgoing)
	cu.Send() <- NewFrame(SystemMsg, []byte{'O'}, Outgoing)
	return nil
}

func (cu *CanusbFTDI) Open(ctx context.Context) error {
	if p, err := ftdi.Open(ftdi.DeviceInfo{
		Index: cu.devIndex,
	}); err != nil {
		return fmt.Errorf("failed to open ftdi device: %w", err)
	} else {
		cu.port = p
		if err := p.SetLineProperty(ftdi.LineProperties{Bits: 8, StopBits: 0, Parity: ftdi.NONE}); err != nil {
			p.Close()
			return err
		}

		if err := p.SetBaudRate(3000000); err != nil {
			p.Close()
			return err
		}

		if err := p.SetLatency(2); err != nil {
			p.Close()
			return err
		}

		if err := p.SetTimeout(10, 10); err != nil {
			p.Close()
			return err
		}
	}

	var cmds = []string{
		"C", "", "", // Empty buffer
		"V", // Get Version number of both CANUSB hardware and software
		//"N",        // Get Serial number of the CANUSB
		"Z0",       // Sets Time Stamp OFF for received frames
		cu.canRate, // Setup CAN bit-rates
		cu.filter,
		cu.mask,
		"O", // Open the CAN channel
	}

	for _, c := range cmds {
		_, err := cu.port.Write([]byte(c + "\r"))
		if err != nil {
			cu.port.Close()
			return err
		}
		time.Sleep(10 * time.Millisecond)
	}

	cu.port.Purge(ftdi.FT_PURGE_BOTH)

	go cu.recvManager(ctx)
	go cu.sendManager(ctx)

	return nil
}

func (cu *CanusbFTDI) setCANrate(rate float64) error {
	switch rate {
	case 10:
		cu.canRate = "S0"
	case 20:
		cu.canRate = "S1"
	case 33.3:
		cu.canRate = "s0e1c"
	case 47.619:
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
	case 615.384:
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

func (cu *CanusbFTDI) Close() error {
	cu.BaseAdapter.Close()
	if cu.port != nil {
		cu.port.Write([]byte("C\r"))
		time.Sleep(100 * time.Millisecond)
		cu.port.Purge(ftdi.FT_PURGE_BOTH)
		if err := cu.port.Close(); err != nil {
			return fmt.Errorf("failed to close com port: %w", err)
		}
		cu.port = nil
	}
	return nil
}

func (*CanusbFTDI) calcAcceptanceFilters(idList []uint32) (string, string) {
	if len(idList) == 1 && idList[0] == 0 {
		return "\r", "\r"
	}
	var code = ^uint32(0)
	var mask uint32 = 0

	if len(idList) == 0 {
		code = 0
		mask = ^uint32(0)
	} else {
		code = 0
		for _, canID := range idList {
			code &= (canID & 0x7FF) << 5
			mask |= (canID & 0x7FF) << 5
		}
	}
	code |= code << 16
	mask |= mask << 16

	return fmt.Sprintf("M%08X", code), fmt.Sprintf("m%08X", mask)
}

func (cu *CanusbFTDI) sendManager(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	idBuff := make([]byte, 4)

	for {
		select {
		case <-ctx.Done():
			return
		case <-cu.closeChan:
			return
		case <-ticker.C:
			cu.sendSem <- struct{}{}
			cu.port.Write([]byte{'F', '\r'})
		case msg := <-cu.sendChan:
			if id := msg.Identifier; id >= SystemMsg {
				if id == SystemMsg {
					if cu.cfg.Debug {
						cu.cfg.OnMessage(">> " + string(msg.Data))
					}
					if _, err := cu.port.Write(append(msg.Data, '\r')); err != nil {
						cu.SetError(Unrecoverable(fmt.Errorf("failed to write to com port: %w", err)))
						return
					}
				}
				continue
			}

			cu.sendSem <- struct{}{}

			var out []byte
			if msg.Extended {
				binary.BigEndian.PutUint32(idBuff, msg.Identifier&0x1FFFFFFF)
				out = []byte("T" + hex.EncodeToString(idBuff) + strconv.Itoa(msg.Length()) + hex.EncodeToString(msg.Data) + "\r")
				if _, err := cu.port.Write(out); err != nil {
					cu.SetError(Unrecoverable(fmt.Errorf("failed to write to com port: %w", err)))
					return
				}
			} else {
				binary.BigEndian.PutUint32(idBuff, msg.Identifier&0x7FF)
				out = []byte("t" + hex.EncodeToString(idBuff)[5:] + strconv.Itoa(msg.Length()) + hex.EncodeToString(msg.Data) + "\r")
				if _, err := cu.port.Write(out); err != nil {
					cu.SetError(Unrecoverable(fmt.Errorf("failed to write to com port: %w", err)))
					return
				}
			}

			if cu.cfg.Debug {
				cu.cfg.OnMessage(">> " + string(out))
			}

		}
	}
}

func (cu *CanusbFTDI) recvManager(ctx context.Context) {
	//readBuffer := make([]byte, 16)
	rx_cnt := int32(16)
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		var err error
		rx_cnt, err = cu.port.GetQueueStatus()
		if err != nil {
			cu.SetError(Unrecoverable(fmt.Errorf("failed to get queue status: %w", err)))
			return
		}
		if rx_cnt == 0 {
			time.Sleep(400 * time.Microsecond)
			continue
		}

		readBuffer := make([]byte, rx_cnt)
		n, err := cu.port.Read(readBuffer)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			cu.SetError(fmt.Errorf("failed to read com port: %w", err))
			return
		}
		select {
		case <-cu.closeChan:
			return
		default:
			if n == 0 {
				continue
			}
			cu.parse(readBuffer[:n])
		}

	}
}

func (cu *CanusbFTDI) parse(data []byte) {
	for _, b := range data {
		if b == 0x07 { // BELL
			cu.SetError(errors.New("command error"))
			select {
			case <-cu.sendSem:
			default:
			}
			continue
		}
		if b != 0x0D { // CR
			cu.buff.WriteByte(b)
			continue
		}
		if cu.buff.Len() == 0 {
			continue
		}
		by := cu.buff.Bytes()
		if cu.cfg.Debug {
			cu.cfg.OnMessage("<< " + cu.buff.String())
		}
		switch by[0] {
		case 'F':
			select {
			case <-cu.sendSem:
			default:
			}
			if err := decodeStatus(by); err != nil {
				cu.SetError(fmt.Errorf("CAN status error: %w", err))
			}
		case 't':
			f, err := cu.decodeFrame(by)
			if err != nil {
				cu.SetError(fmt.Errorf("failed to decode frame: %w", err))
				cu.buff.Reset()
				continue
			}
			select {
			case cu.recvChan <- f:
			default:
				cu.SetError(ErrDroppedFrame)
			}
			cu.buff.Reset()
		case 'T':
			f, err := cu.decodeExtendedFrame(by)
			if err != nil {
				cu.SetError(fmt.Errorf("failed to decode frame: %w", err))
				cu.buff.Reset()
				continue
			}
			select {
			case cu.recvChan <- f:
			default:
				cu.SetError(ErrDroppedFrame)
			}
			cu.buff.Reset()
		case 'z': // last command ok
			select {
			case <-cu.sendSem:
			default:
			}
		case 'V':
			if cu.cfg.PrintVersion {
				cu.cfg.OnMessage("H/W version " + cu.buff.String())
			}
		case 'N':
			if cu.cfg.PrintVersion {
				cu.cfg.OnMessage("H/W serial " + cu.buff.String())
			}
		default:
			cu.cfg.OnMessage("Unknown>> " + cu.buff.String())
		}
		cu.buff.Reset()
	}
}

func (*CanusbFTDI) decodeFrame(buff []byte) (*CANFrame, error) {
	id, err := strconv.ParseUint(string(buff[1:4]), 16, 32)
	if err != nil {
		return nil, fmt.Errorf("failed to decode identifier: %v", err)
	}

	/* leng, err := hex.DecodeString("0" + string(buff[4]))
	if err != nil {
		return nil, fmt.Errorf("failed to decode message length: %v", err)
	}
	msgLen := int(leng[0])
	if msgLen > 8 {
		log.Println("msgLen", msgLen)
	} */

	//data, err := hex.DecodeString(string(buff[5 : 5+(msgLen*2)]))

	data, err := hex.DecodeString(string(buff[5:]))
	if err != nil {
		return nil, fmt.Errorf("failed to decode body: %v", err)
	}
	return NewFrame(
		uint32(id),
		data,
		Incoming,
	), nil
}

// T 00000180 8 2D 12 09 DF 87 56 91 06
func (*CanusbFTDI) decodeExtendedFrame(buff []byte) (*CANFrame, error) {
	id, err := strconv.ParseUint(string(buff[1:9]), 16, 32)
	if err != nil {
		return nil, fmt.Errorf("failed to decode identifier: %v", err)
	}

	/* leng, err := hex.DecodeString("0" + string(buff[4]))
	if err != nil {
		return nil, fmt.Errorf("failed to decode message length: %v", err)
	}
	msgLen := int(leng[0])
	if msgLen > 8 {
		log.Println("msgLen", msgLen)
	} */

	//data, err := hex.DecodeString(string(buff[5 : 5+(msgLen*2)]))

	data, err := hex.DecodeString(string(buff[10:]))
	if err != nil {
		return nil, fmt.Errorf("failed to decode body: %v", err)
	}
	return NewExtendedFrame(
		uint32(id),
		data,
		Incoming,
	), nil
}

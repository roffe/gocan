//go:build ftdi

package gocan

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	ftdi "github.com/roffe/goftdi"
)

type STNFTDI struct {
	BaseAdapter

	baseName string

	devIndex uint64

	port *ftdi.Device

	canrateCMD   string
	protocolCMD  string
	filter, mask string

	sendSem chan struct{}
}

func NewSTNFTDI(name string, idx uint64) func(cfg *AdapterConfig) (Adapter, error) {
	return func(cfg *AdapterConfig) (Adapter, error) {
		stn := &STNFTDI{
			BaseAdapter: NewBaseAdapter(name, cfg),
			devIndex:    idx,
			sendSem:     make(chan struct{}, 1),
			baseName:    strings.TrimPrefix(name, "d2xx "),
		}

		protocolCMD, canrateCMD, err := scantoolCalculateCANrate(stn.baseName, cfg.CANRate)
		if err != nil {
			return nil, err
		}
		stn.protocolCMD = protocolCMD
		stn.canrateCMD = canrateCMD

		stn.filter, stn.mask = scantoolCANfilter(cfg.CANFilter)

		return stn, nil
	}
}

func (stn *STNFTDI) SetFilter(filters []uint32) error {
	stn.filter, stn.mask = scantoolCANfilter(stn.cfg.CANFilter)
	stn.Send() <- NewFrame(SystemMsg, []byte("STPC"), Outgoing)
	stn.Send() <- NewFrame(SystemMsg, []byte(stn.mask), Outgoing)
	stn.Send() <- NewFrame(SystemMsg, []byte(stn.filter), Outgoing)
	stn.Send() <- NewFrame(SystemMsg, []byte("STPO"), Outgoing)
	return nil
}

func (stn *STNFTDI) Open(ctx context.Context) error {

	if p, err := ftdi.Open(ftdi.DeviceInfo{
		Index: stn.devIndex,
	}); err != nil {
		return fmt.Errorf("failed to open ftdi device: %w", err)
	} else {
		stn.port = p
		if err := p.SetLineProperty(ftdi.LineProperties{Bits: 8, StopBits: 0, Parity: ftdi.NONE}); err != nil {
			p.Close()
			return err
		}

		if err := p.SetLatency(1); err != nil {
			p.Close()
			return err
		}

		if err := p.SetTimeout(10, 10); err != nil {
			p.Close()
			return err
		}
	}

	to := uint(2000000)
	found := false
	for _, from := range [...]uint{115200, 38400, 230400, 921600, 2000000, 1000000, 57600} {
		log.Println("trying to change baudrate from", from, "to", to, "bps")
		if stn.trySpeed(from, to) == nil {
			found = true
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if !found {
		stn.port.Close()
		return errors.New("failed to switch adapter baudrate")
	}

	var initCmds = []string{
		"ATE0",          // turn off echo
		"STUFC0",        // Turn on flow control
		"ATS0",          // turn off spaces
		"ATV1",          // variable DLC on
		stn.protocolCMD, // Set canbus protocol
		"ATH1",          // Headers on
		"ATAT0",         // Set adaptive timing mode off
		"ATCAF0",        // Automatic formatting off
		stn.canrateCMD,  // Set CANrate
		//"ATAL",          // Allow long messages
		"ATCFC0", //Turn automatic CAN flow control off
		//"ATAR",      // Automatically set the receive address.
		//"ATCSM0",  //Turn CAN silent monitoring off
		//"STCMM1",   // Set CAN monitor monitor - Normal node – with CAN ACKs
		"ATST32",   // Set timeout to 200msec
		"ATR0",     // Turn off replies
		stn.mask,   // mask
		stn.filter, // code
	}

	delay := 20 * time.Millisecond

	for _, cmd := range initCmds {
		if cmd == "" {
			continue
		}
		out := []byte(cmd + "\r")
		if stn.cfg.Debug {
			stn.cfg.OnMessage(">> " + cmd)
		}
		if _, err := stn.port.Write(out); err != nil {
			stn.cfg.OnMessage(err.Error())
		}
		time.Sleep(delay)
	}

	stn.port.Purge(ftdi.FT_PURGE_BOTH)

	go stn.sendManager(ctx)
	go stn.recvManager(ctx)

	return nil
}

func (stn *STNFTDI) trySpeed(from, to uint) error {

	if err := stn.port.SetBaudRate(uint(from)); err != nil {
		return err
	}

	if _, err := stn.port.Write([]byte("\r\r\r")); err != nil {
		return err
	}

	time.Sleep(20 * time.Millisecond)

	if _, err := stn.port.Write([]byte("STBR" + strconv.Itoa(int(to)) + "\r")); err != nil {
		return err
	}
	time.Sleep(10 * time.Millisecond)

	if err := stn.port.Purge(ftdi.FT_PURGE_RX); err != nil {
		return err
	}
	if err := stn.port.SetBaudRate(uint(to)); err != nil {
		return err
	}

	buff := bytes.NewBuffer(nil)
	defer buff.Reset()

	var readbuff = make([]byte, 16)
	for range 10 {
		n, err := stn.port.Read(readbuff)
		if err != nil {
			return err
		}
		if n == 0 {
			time.Sleep(4 * time.Millisecond)
			continue
		}
		for _, b := range readbuff[:n] {
			if b == 0x0D {
				if buff.Len() == 0 {
					continue
				}
				if bytes.Contains(buff.Bytes(), []byte("STN")) {
					stn.cfg.OnMessage(buff.String())
					if _, err := stn.port.Write([]byte("\r")); err != nil {
						return err
					}
					//stn.cfg.OnMessage(fmt.Sprintf("baudrate changed to %d bps", to))
					return nil
				}
				buff.Reset()
				continue
			}
			buff.WriteByte(b)
		}
	}

	return fmt.Errorf("failed to change adapter baudrate from %d to %d bps", from, to)
}

func (stn *STNFTDI) Close() error {
	stn.BaseAdapter.Close()
	time.Sleep(50 * time.Millisecond)
	stn.port.Write([]byte("ATZ\r"))
	time.Sleep(100 * time.Millisecond)
	stn.port.Purge(ftdi.FT_PURGE_BOTH)
	return stn.port.Close()
}

func (stn *STNFTDI) sendManager(ctx context.Context) {
	f := bytes.NewBuffer(nil)
	idb := make([]byte, 4)
	for {
		select {
		case v := <-stn.sendChan:
			if id := v.Identifier; id >= SystemMsg {
				if id == SystemMsg {
					if stn.cfg.Debug {
						stn.cfg.OnMessage("<o> " + f.String())
					}
					stn.sendSem <- struct{}{}
					if _, err := stn.port.Write(append(v.Data, '\r')); err != nil {
						stn.SetError(Unrecoverable(fmt.Errorf("failed to write: %q %w", f.String(), err)))
						return
					}
				}
				continue
			}
			binary.BigEndian.PutUint32(idb, v.Identifier)
			f.WriteString("STPXh:" + hex.EncodeToString(idb)[5:] + ",d:" + hex.EncodeToString(v.Data))
			if v.Timeout != 0 && v.Timeout != 200 {
				f.WriteString(",t:" + strconv.Itoa(int(v.Timeout)))
			}
			// timeout = 0
			respCount := v.FrameType.Responses
			if respCount > 0 {
				f.WriteString(",r:" + strconv.Itoa(respCount))
			}
			f.WriteString("\r")
			if stn.cfg.Debug {
				stn.cfg.OnMessage("<o> " + f.String())
			}
			stn.sendSem <- struct{}{}
			if _, err := stn.port.Write(f.Bytes()); err != nil {
				stn.SetError(Unrecoverable(fmt.Errorf("failed to write: %q %w", f.String(), err)))
				return
			}
			f.Reset()
		case <-ctx.Done():
			return
		case <-stn.closeChan:
			return
		}
	}
}

func (stn *STNFTDI) recvManager(ctx context.Context) {
	buff := bytes.NewBuffer(nil)
	rx_cnt := int32(16)
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		var err error
		rx_cnt, err = stn.port.GetQueueStatus()
		if err != nil {
			stn.SetError(Unrecoverable(fmt.Errorf("failed to get queue status: %w", err)))
			return
		}
		if rx_cnt == 0 {
			time.Sleep(400 * time.Microsecond)
			continue
		}

		readBuffer := make([]byte, rx_cnt)
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

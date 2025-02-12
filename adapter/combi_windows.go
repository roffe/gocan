//go:build combi

package adapter

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/google/gousb"
	"github.com/roffe/gocan"
)

const (
	combiVid              = 0xFFFF
	combiPid              = 0x0005
	combiCmdtxFrame       = 0x83
	combiCmdrxFrame       = 0x82
	combiCmdSetCanBitrate = 0x81
	combiCmdOpen          = 0x80
	combiCmdVersion       = 0x20
)

type CombiAdapter struct {
	BaseAdapter
	usbCtx    *gousb.Context
	dev       *gousb.Device
	devCfg    *gousb.Config
	iface     *gousb.Interface
	in        *gousb.InEndpoint
	out       *gousb.OutEndpoint
	sendSem   chan struct{}
	cmdBuffer []byte
	closeOnce sync.Once
	buffPool  sync.Pool
}

func init() {
	//ctx := gousb.NewContext()
	//defer ctx.Close()
	//dev, err := ctx.OpenDeviceWithVIDPID(combiVid, combiPid)
	//if err != nil || dev == nil {
	//	return
	//}
	//defer dev.Close()
	if err := Register(&AdapterInfo{
		Name:               "CombiAdapter",
		Description:        "libusb windows driver",
		RequiresSerialPort: false,
		Capabilities: AdapterCapabilities{
			HSCAN: true,
			KLine: false,
			SWCAN: false,
		},
		New: NewCombi,
	}); err != nil {
		panic(err)
	}
}

func NewCombi(cfg *gocan.AdapterConfig) (gocan.Adapter, error) {
	return &CombiAdapter{
		BaseAdapter: NewBaseAdapter("CombiAdapter", cfg),
		sendSem:     make(chan struct{}, 1),
		buffPool: sync.Pool{
			New: func() any {
				return make([]byte, 19)
			},
		},
	}, nil
}

func (ca *CombiAdapter) SetFilter(filters []uint32) error {
	return nil
}

func (ca *CombiAdapter) Connect(ctx context.Context) error {
	var err error
	ca.usbCtx = gousb.NewContext()
	ca.dev, err = ca.usbCtx.OpenDeviceWithVIDPID(combiVid, combiPid)
	if err != nil {
		if ca.dev == nil {
			ca.closeAdapter(false, false, false, false, true)
			if ca.dev == nil {
				return errors.New("CombiAdapter not found")
			}
			return err
		} else {
			ca.closeAdapter(false, false, false, true, true)
			return err
		}
	}

	if err := ca.dev.SetAutoDetach(true); err != nil {
		ca.cfg.OnError(fmt.Errorf("failed to set auto detach: %w", err))
	}

	ca.devCfg, err = ca.dev.Config(1)
	if err != nil {
		ca.closeAdapter(false, false, false, true, true)
		return err
	}

	ca.iface, err = ca.devCfg.Interface(1, 0)
	if err != nil {
		ca.closeAdapter(false, false, true, true, true)
		return err
	}

	ca.in, err = ca.iface.InEndpoint(2)
	if err != nil {
		ca.cfg.OnError(fmt.Errorf("InEndpoint(2): %w", err))
		ca.closeAdapter(false, true, true, true, true)
		return err
	}

	ca.out, err = ca.iface.OutEndpoint(5)
	if err != nil {
		ca.cfg.OnError(fmt.Errorf("OutEndpoint(5): %w", err))
		ca.cfg.OnMessage("trying EP 2 (stm32 clone)")
		ca.out, err = ca.iface.OutEndpoint(2)
		if err != nil {
			ca.closeAdapter(false, true, true, true, true)
			return err
		}
	}

	// Close can-bus
	if err := ca.canCtrl(0); err != nil {
		ca.closeAdapter(false, true, true, true, true)
		return fmt.Errorf("failed to close can-bus: %w", err)
	}

	dump := make([]byte, 38)
	ca.in.Read(dump)

	if ca.cfg.PrintVersion {
		if ver, err := ca.ReadVersion(ctx); err == nil {
			ca.cfg.OnMessage(ver)
		}
	}

	// Set can bitrate
	if err := ca.setBitrate(ctx); err != nil {
		ca.closeAdapter(false, true, true, true, true)
		return err
	}
	// Open can-bus
	if err := ca.canCtrl(1); err != nil {
		ca.closeAdapter(false, true, true, true, true)
		return fmt.Errorf("failed to open can-bus: %w", err)
	}

	go ca.sendManager()
	go ca.recvManager()

	return nil
}

func (ca *CombiAdapter) canCtrl(mode byte) error {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if _, err := ca.out.WriteContext(ctx, []byte{combiCmdOpen, 0, 1, mode, 0}); err != nil {
		return fmt.Errorf("failed to write to usb device: %w", err)
	}
	return nil
}

func (ca *CombiAdapter) Close() error {
	ca.BaseAdapter.Close()
	var err error
	ca.closeOnce.Do(func() {
		err = ca.closeAdapter(true, true, true, true, true)
	})
	return err
}

func (ca *CombiAdapter) closeAdapter(sendCloseCMD, closeIface, closeDevCfg, closeDev, closeCTX bool) error {
	if sendCloseCMD {
		if err := ca.canCtrl(0); err != nil {
			ca.cfg.OnError(fmt.Errorf("failed to close can-bus: %w", err))
		}
		time.Sleep(50 * time.Millisecond)
	}

	if closeIface && ca.iface != nil {
		ca.iface.Close()
	}

	if closeDevCfg && ca.devCfg != nil {
		if err := ca.devCfg.Close(); err != nil {
			ca.cfg.OnError(fmt.Errorf("failed to close device config: %w", err))
		}
	}
	if closeDev && ca.dev != nil {
		if err := ca.dev.Close(); err != nil {
			ca.cfg.OnError(fmt.Errorf("failed to close device: %w", err))
		}
	}
	if closeCTX && ca.usbCtx != nil {
		if err := ca.usbCtx.Close(); err != nil {
			ca.cfg.OnError(fmt.Errorf("failed to close usb context: %w", err))
		}
	}
	return nil
}

func (ca *CombiAdapter) recvManager() {
	if ca.cfg.Debug {
		defer log.Println("recvManager exited")
	}
	for {
		select {
		case <-ca.close:
			return
		default:
			buff := make([]byte, 19)
			n, err := ca.in.Read(buff)
			if err != nil {
				ca.SetError(fmt.Errorf("failed to read from usb device: %w", err))
				//ca.cfg.OnError(fmt.Errorf("failed to read from usb device: %w", err))
				if n == 0 {
					continue
				}
			}
			if err := ca.parseCMD(buff[:n]); err != nil {
				ca.cfg.OnError(fmt.Errorf("failed to parse command: %w", err))
				continue
			}
		}
	}
}

func (ca *CombiAdapter) parseCMD(data []byte) error {
	// If there's a partial transfer from previous call, concatenate it with the new data
	if len(ca.cmdBuffer) > 0 {
		data = append(ca.cmdBuffer, data...)
		ca.cmdBuffer = nil
	}

	for len(data) > 0 {
		// Check if there's enough data to form a complete command
		if data[0] == combiCmdrxFrame && len(data) < 19 || len(data) < 4 {
			ca.cmdBuffer = data
			break
		}

		// Get size
		size := binary.BigEndian.Uint16(data[1:3])

		// Check if there's enough data to form the complete command including data and terminator
		if len(data) < int(size+4) { // cmd ID(1) + size(2) + terminator(1)
			ca.cmdBuffer = data
			break
		}

		switch data[0] {
		case combiCmdtxFrame, combiCmdSetCanBitrate, combiCmdOpen, combiCmdVersion:
			select {
			case <-ca.sendSem:
			default:
			}
		case combiCmdrxFrame:
			frame := gocan.NewFrame(
				binary.LittleEndian.Uint32(data[3:7]),
				data[7:data[15]+7],
				gocan.Incoming,
			)
			select {
			case ca.recv <- frame:
			default:
				ca.cfg.OnError(ErrDroppedFrame)
			}
		}

		// Advance to next transfer
		data = data[size+4:]
	}

	return nil
}

func (ca *CombiAdapter) sendManager() {
	if ca.cfg.Debug {
		defer log.Println("sendManager exited")
	}
	for {
		select {
		case <-ca.close:
			return
		case frame := <-ca.send:
			if frame.Identifier() >= gocan.SystemMsg {
				continue
			}
			ca.sendMessage(frame)
		}
	}
}

func (ca *CombiAdapter) sendMessage(frame gocan.CANFrame) {
	buff := ca.buffPool.Get().([]byte)
	defer ca.buffPool.Put(buff)
	buff[0] = combiCmdtxFrame
	//buff[1] = 15 >> 8
	//buff[2] = 15 & 0xff
	buff[1] = 0x00
	buff[2] = 0x0F
	binary.LittleEndian.PutUint32(buff[3:], frame.Identifier())
	copy(buff[7:], frame.Data())
	buff[15] = uint8(frame.Length())
	buff[16] = 0x00 // is extended
	buff[17] = 0x00 // is remote
	buff[18] = 0x00 // terminator
	ca.sendSem <- struct{}{}
	if _, err := ca.out.Write(buff); err != nil {
		ca.SetError(fmt.Errorf("failed to send frame: %w", err))
	}
}

func (ca *CombiAdapter) setBitrate(ctx context.Context) error {
	var canrate uint32
	if ca.cfg.CANRate == 615.384 {
		canrate = 615000
	} else {
		canrate = uint32(ca.cfg.CANRate * 1000)
	}

	payload := []byte{combiCmdSetCanBitrate, 0x00, 0x04, byte(canrate >> 24), byte(canrate >> 16), byte(canrate >> 8), byte(canrate), 0x00}

	wctx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()
	if _, err := ca.out.WriteContext(wctx, payload); err != nil {
		ca.cfg.OnError(fmt.Errorf("failed to set bitrate: %w", err))
		return err
	}
	return nil
}

func (ca *CombiAdapter) ReadVersion(ctx context.Context) (string, error) {
	rctx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()
	if _, err := ca.out.WriteContext(rctx, []byte{combiCmdVersion, 0x00, 0x00, 0x00}); err != nil {
		return "", err
	}
	vers := make([]byte, ca.in.Desc.MaxPacketSize)
	_, err := ca.in.Read(vers)
	if err != nil {
		return "", err
	}
	//  20 00 02 01 01 00
	return fmt.Sprintf("CombiAdapter v%d.%d", vers[4], vers[3]), nil
}

/*
func frameToPacket(frame gocan.CANFrame) *CombiPacket {
	buff := make([]byte, 15)
	binary.LittleEndian.PutUint32(buff, frame.Identifier())
	copy(buff[4:], frame.Data())
	buff[12] = uint8(frame.Length())
	buff[13] = 0
	buff[14] = 0
	return &CombiPacket{
		cmd:  cmdtxFrame,
		len:  15,
		data: buff,
		term: 0x00,
	}
}
*/
/*
func (ca *CombiAdapter) sendFrame(ctx context.Context, frame gocan.CANFrame) error {
	buff := make([]byte, 15)
	binary.LittleEndian.PutUint32(buff, frame.Identifier())
	copy(buff[4:], frame.Data())
	buff[12] = uint8(frame.Length())
	buff[13] = 0
	buff[14] = 0
	tx := &CombiPacket{
		cmd:  cmdtxFrame,
		len:  15,
		data: buff,
		term: 0x00,
	}
	b := tx.Bytes()
	ca.sendSem <- struct{}{}
	n, err := ca.out.Write(b)
	if n != len(b) {
		ca.cfg.OnError(fmt.Errorf("sent %d bytes of data out of %d", n, len(b)))
	}
	if err != nil {
		return err
	}
	return nil
}

type CombiPacket struct {
	cmd  uint8
	len  uint16
	data []byte
	term uint8
}

func (cp *CombiPacket) Bytes() []byte {
	buf := new(bytes.Buffer)
	buf.WriteByte(cp.cmd)
	buf.Write([]byte{uint8(cp.len >> 8), uint8(cp.len & 0xff)})
	if cp.data != nil {
		buf.Write(cp.data)
	}
	buf.WriteByte(cp.term)
	return buf.Bytes()
}

func (ca *CombiAdapter) recvManager() {
	go ca.ringManager()
	dataBuff := make([]byte, 32)
	for {
		select {
		case <-ca.close:
			return
		default:
			if ca.rb.IsEmpty() {
				continue
			}
			cmd, err := ca.rb.ReadByte()
			if err != nil {
				ca.cfg.OnError(fmt.Errorf("failed to read cmd from ringbuffer: %w", err))
				continue
			}

			switch cmd {
			case combiCmdrxFrame:
				for ca.rb.Length() < 18 {
					//log.Println("waiting for rx data length")
					time.Sleep(10 * time.Microsecond)
				}
			case combiCmdtxFrame, combiCmdVersion, combiCmdOpen:
				select {
				case <-ca.sendSem:
				default:
				}
				fallthrough
			default:
				for ca.rb.Length() < 3 {
					//log.Println("waiting for command data")
					time.Sleep(10 * time.Microsecond)
				}
			}

			if _, err := ca.rb.Read(dataBuff[:2]); err != nil {
				ca.cfg.OnError(fmt.Errorf("failed to read length from ringbuffer: %w", err))
			}
			dataLen := int(binary.BigEndian.Uint16(dataBuff[:2]) + 0x01) // +1 for terminator

			n, err := ca.rb.Read(dataBuff[:dataLen])
			if err != nil {
				ca.cfg.OnError(fmt.Errorf("failed to read data from ringbuffer: %w", err))
			}
			if n != dataLen {
				ca.cfg.OnError(fmt.Errorf("read %d bytes, expected %d", n, dataLen))
			}

			switch cmd {
			case combiCmdrxFrame: //rx
				ca.recv <- gocan.NewFrame(
					binary.LittleEndian.Uint32(dataBuff[:4]),
					dataBuff[4:4+dataBuff[12]],
					gocan.Incoming,
				)
			}

			if ca.cfg.Debug {
				log.Printf("cmd: %02X, len: %d, data: %X, term: %02X", cmd, dataLen, dataBuff[:dataLen-2], dataBuff[dataLen-1])
			}
		}
	}
}
*/

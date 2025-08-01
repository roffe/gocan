//go:build combi

package gocan

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"math"
	"sync"
	"time"

	"github.com/google/gousb"
	"github.com/google/gousb/usbid"
)

const (
	combiVid = 0xFFFF
	combiPid = 0x0005
)

var (
	ErrInvalidCommand      = errors.New("invalid command")
	ErrCommandSizeTooLarge = errors.New("command size too large")
	ErrCommandTermination  = errors.New("command terminated with NAK")
)

const (
	MaxCommandSize = 1024
)

const (
	term_ack = 0x00 ///< command acknowledged
	term_nak = 0xFF ///< command failed

	cmd_brd_fwversion = 0x20 ///< firmware version
	cmd_brd_adcfilter = 0x21 ///< ADC filter settings
	cmd_brd_adc       = 0x22 ///< ADC value
	cmd_brd_egt       = 0x23 ///< EGT value

	cmd_bdm_stop        = 0x40 ///< stop chip
	cmd_bdm_reset       = 0x41 ///< reset chip
	cmd_bdm_run         = 0x42 ///< run from given address
	cmd_bdm_step        = 0x43 ///< step chip
	cmd_bdm_restart     = 0x44 ///< restart
	cmd_bdm_readmem     = 0x45 ///< read memory
	cmd_bdm_writemem    = 0x46 ///< write memory
	cmd_bdm_readsysreg  = 0x47 ///< read system register
	cmd_bdm_writesysreg = 0x48 ///< write system register
	cmd_bdm_readadreg   = 0x49 ///< read A/D register
	cmd_bdm_writeadreg  = 0x4a ///< write A/D register
	cmd_bdm_readflash   = 0x4b ///< read ECU flash
	cmd_bdm_eraseflash  = 0x4c ///< erase ECU flash
	cmd_bdm_writeflash  = 0x4d ///< write ECU flash
	cmd_bdm_pinstate    = 0x4e ///< BDM pin state

	cmd_can_open       = 0x80 ///< open/close CAN channel
	cmd_can_bitrate    = 0x81 ///< set bitrate
	cmd_can_frame      = 0x82 ///< incoming frame
	cmd_can_txframe    = 0x83 ///< outgoing frame
	cmd_can_ecuconnect = 0x89 ///< connect / disconnect ECU
	cmd_can_readflash  = 0x8a ///< read ECU flash
	cmd_can_writeflash = 0x8b ///< write ECU flash

	cmd_mw_readall  = 0xa0
	cmd_mw_writeall = 0xa1
	cmd_mw_eraseall = 0xa2
)

const (
	stepCommand = iota
	stepSizeHigh
	stepSizeLow
	stepData
	stepTermination
)

type CombiAdapter struct {
	BaseAdapter
	usbCtx    *gousb.Context
	dev       *gousb.Device
	devCfg    *gousb.Config
	iface     *gousb.Interface
	in        *gousb.InEndpoint
	out       *gousb.OutEndpoint
	sendSem   chan byte
	cmdBuffer []byte
	closeOnce sync.Once
	txPool    sync.Pool

	adcFilterChan   chan bool
	adcValueChan    chan float32
	thermoValueChan chan float32
}

func init() {
	//list()
	//ctx := gousb.NewContext()
	//defer ctx.Close()
	//dev, err := ctx.OpenDeviceWithVIDPID(combiVid, combiPid)
	//if err != nil || dev == nil {
	//	return
	//}
	//defer dev.Close()
	if err := RegisterAdapter(&AdapterInfo{
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

func NewCombi(cfg *AdapterConfig) (Adapter, error) {
	return &CombiAdapter{
		BaseAdapter:     NewBaseAdapter("CombiAdapter", cfg),
		sendSem:         make(chan byte, 1),
		adcFilterChan:   make(chan bool, 1),
		adcValueChan:    make(chan float32),
		thermoValueChan: make(chan float32, 1),
		txPool: sync.Pool{
			New: func() any {
				return []byte{cmd_can_txframe, 0x00, 0x0F, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
			},
		},
	}, nil
}

func (ca *CombiAdapter) SetFilter(filters []uint32) error {
	return nil
}

func (ca *CombiAdapter) Open(ctx context.Context) error {
	var err error
	ca.usbCtx = gousb.NewContext()
	ca.dev, err = ca.usbCtx.OpenDeviceWithVIDPID(combiVid, combiPid)
	if err != nil {
		if ca.dev == nil {
			return errors.New("CombiAdapter not found")
		} else {
			ca.dev.Close()
			ca.usbCtx.Close()
			return err
		}
	}

	if ca.dev == nil {
		ca.usbCtx.Close()
		return errors.New("CombiAdapter not found 2")
	}

	//if err := ca.dev.SetAutoDetach(true); err != nil {
	//	ca.cfg.OnMessage(fmt.Sprintf("failed to set auto detach: %v", err))
	//}

	ca.devCfg, err = ca.dev.Config(1)
	if err != nil {
		if ca.devCfg != nil {
			ca.devCfg.Close()
		}
		ca.dev.Close()
		ca.usbCtx.Close()
		return err
	}

	ca.iface, err = ca.devCfg.Interface(1, 0)
	if err != nil {
		if ca.iface != nil {
			ca.iface.Close()
		}
		ca.devCfg.Close()
		ca.dev.Close()
		ca.usbCtx.Close()
		return err
	}

	ca.in, err = ca.iface.InEndpoint(2)
	if err != nil {
		ca.SetError(fmt.Errorf("InEndpoint(2): %w", err))
		ca.iface.Close()
		ca.devCfg.Close()
		ca.dev.Close()
		ca.usbCtx.Close()
		return err
	}

	ca.out, err = ca.iface.OutEndpoint(5)
	if err != nil {
		ca.SetError(fmt.Errorf("OutEndpoint(5): %w", err))
		ca.cfg.OnMessage("trying EP 2 (stm32 clone)")
		ca.out, err = ca.iface.OutEndpoint(2)
		if err != nil {
			ca.iface.Close()
			ca.devCfg.Close()
			ca.dev.Close()
			ca.usbCtx.Close()
			return err
		}
	}

	// Close canbus
	if err := ca.canCtrl(0); err != nil {
		ca.iface.Close()
		ca.devCfg.Close()
		ca.dev.Close()
		ca.usbCtx.Close()
		return fmt.Errorf("failed to close canbus: %w", err)
	}

	dump := make([]byte, 1024)
	ca.in.ReadContext(ctx, dump)

	if ca.cfg.PrintVersion {
		if ver, err := ca.ReadVersion(ctx); err == nil {
			ca.cfg.OnMessage(ver)
		}
	}

	if ca.cfg.AdditionalConfig["NoConnect"] != "true" {
		// Set can bitrate
		if err := ca.setBitrate(ctx); err != nil {
			ca.iface.Close()
			ca.devCfg.Close()
			ca.dev.Close()
			ca.usbCtx.Close()
			return err
		}

		// Open canbus
		if err := ca.canCtrl(1); err != nil {
			ca.iface.Close()
			ca.devCfg.Close()
			ca.dev.Close()
			ca.usbCtx.Close()
			return fmt.Errorf("failed to open canbus: %w", err)
		}
	}
	go ca.sendManager(ctx)
	go ca.recvManager(ctx)

	return nil
}

func (ca *CombiAdapter) Close() error {
	ca.BaseAdapter.Close()
	var err error
	ca.closeOnce.Do(func() {
		err = ca.closeAdapter()
	})
	return err
}

func (ca *CombiAdapter) ReadVersion(ctx context.Context) (string, error) {
	rctx, cancel := context.WithTimeout(ctx, 20*time.Millisecond)
	defer cancel()
	if _, err := ca.out.WriteContext(rctx, []byte{cmd_brd_fwversion, 0x00, 0x00, 0x00}); err != nil {
		return "", err
	}
	vers := make([]byte, ca.in.Desc.MaxPacketSize)
	_, err := ca.in.ReadContext(ctx, vers)
	if err != nil {
		return "", err
	}
	//  20 00 02 01 01 00
	return fmt.Sprintf("CombiAdapter v%d.%d", vers[4], vers[3]), nil
}

func (ca *CombiAdapter) GetADCFiltering(ctx context.Context, channel int) (bool, error) {
	if err := ca.sendCommand(ctx, cmd_brd_adcfilter, []byte{byte(channel)}, 10); err != nil {
		return false, err
	}
	select {
	case b := <-ca.adcFilterChan:
		return b, nil
	case <-time.After(5 * time.Millisecond):
		return false, fmt.Errorf("timeout waiting for ADC filter response")
	}
}

func (ca *CombiAdapter) SetADCFiltering(ctx context.Context, channel int, enabled bool) error {
	var enableByte byte
	if enabled {
		enableByte = 0x01
	}
	return ca.sendCommand(ctx, cmd_brd_adcfilter, []byte{byte(channel), enableByte}, 10)
}

func (ca *CombiAdapter) GetADCValue(ctx context.Context, channel int) (float64, error) {
	//ca.sendChan <- &CANFrame{
	//	Identifier: SystemMsg,
	//	Data:       []byte{cmd_brd_adc, 0x00, 0x01, byte(channel), 0x00},
	//}

	if err := ca.sendCommand(ctx, cmd_brd_adc, []byte{byte(channel)}, 10); err != nil {
		return 0, fmt.Errorf("failed to send ADC value request: %w", err)
	}

	select {
	case f := <-ca.adcValueChan:
		return float64(f), nil
	case <-time.After(200 * time.Millisecond):
		return 0, fmt.Errorf("timeout waiting for ADC value response")
	}
}

func (ca *CombiAdapter) GetThermoValue(ctx context.Context) (float32, error) {
	ca.sendChan <- &CANFrame{
		Identifier: SystemMsg,
		Data:       []byte{cmd_brd_egt, 0x00, 0x00, 0x00},
	}
	select {
	case f := <-ca.thermoValueChan:
		return f, nil
	case <-time.After(5 * time.Millisecond):
		return 0, fmt.Errorf("timeout waiting for thermo value response")
	}
}

func (ca *CombiAdapter) sendCommand(ctx context.Context, cmd byte, data []byte, timeout int) error {
	if len(data) > MaxCommandSize {
		return fmt.Errorf("%w: %d", ErrCommandSizeTooLarge, len(data))
	}

	select {
	case ca.sendSem <- cmd:
	case <-ctx.Done():
		return ctx.Err()
	}

	gctx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Millisecond)
	defer cancel()

	length := len(data)
	payload := make([]byte, 3+length+1) // cmd + size (2 bytes) + data + terminator
	payload[0] = cmd
	payload[1] = byte(length >> 8)   // size high byte
	payload[2] = byte(length & 0xff) // size low byte
	copy(payload[3:], data)
	payload[3+length] = term_ack // terminator

	if _, err := ca.out.WriteContext(gctx, payload); err != nil {
		return fmt.Errorf("failed to send cmd %2X %2X: %w", cmd, data, err)
	}
	return nil
}

func (ca *CombiAdapter) closeAdapter() error {
	ca.sendSem <- cmd_can_open
	if err := ca.canCtrl(0); err != nil {
		ca.cfg.OnMessage(fmt.Sprintf("failed to close canbus: %v", err))
	}
	time.Sleep(50 * time.Millisecond)

	if ca.iface != nil {
		ca.iface.Close()
	}

	if ca.devCfg != nil {
		if err := ca.devCfg.Close(); err != nil {
			ca.cfg.OnMessage(fmt.Sprintf("failed to close device config: %v", err))
		}
	}
	if ca.dev != nil {
		if err := ca.dev.Close(); err != nil {
			ca.cfg.OnMessage(fmt.Sprintf("failed to close device: %v", err))
		}
	}
	if ca.usbCtx != nil {
		if err := ca.usbCtx.Close(); err != nil {
			ca.cfg.OnMessage(fmt.Sprintf("failed to close usb context: %v", err))
		}
	}
	return nil
}

var combiValidCommands = map[byte]struct{}{
	cmd_can_open:    {},
	cmd_can_bitrate: {},
	cmd_can_frame:   {},
	cmd_can_txframe: {},

	cmd_brd_fwversion: {},
	cmd_brd_adcfilter: {},
	cmd_brd_adc:       {},
	cmd_brd_egt:       {},
}

func (ca *CombiAdapter) recvManager(ctx context.Context) {
	if ca.cfg.Debug {
		defer log.Println("recvManager exited")
	}
	var readBuff [MaxCommandSize]byte
	var parseStep int
	var command byte
	var commandSize uint16
	var commandData [MaxCommandSize]byte
	var commandPointer uint16

	for {
		select {
		case <-ctx.Done():
			return
		case <-ca.closeChan:
			return
		default:
			n, err := ca.in.ReadContext(ctx, readBuff[:])
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				//ca.cfg.OnMessage(fmt.Sprintf("failed to read from usb device: %v", err))
				ca.SetError(fmt.Errorf("failed to read from usb device: %w", err))
				if n == 0 {
					continue
				}
			}
			//if n == 0 {
			//	time.Sleep(5 * time.Millisecond)
			//	continue
			//}
			for _, b := range readBuff[:n] {
				switch parseStep {
				case stepCommand:
					if _, valid := combiValidCommands[b]; !valid {
						ca.SetError(fmt.Errorf("%w: %02X", ErrInvalidCommand, b))
						parseStep = stepCommand // Explicit reset
						continue
					}
					command = b
					commandPointer = 0
					commandSize = 0
					parseStep++
				case stepSizeHigh:
					commandSize = uint16(b) << 8
					parseStep++
				case stepSizeLow:
					commandSize |= uint16(b)
					if commandSize >= 1024 {
						ca.SetError(fmt.Errorf("command size too large: %d", commandSize))
						parseStep = stepCommand
						continue
					}
					if commandSize == 0 {
						parseStep = stepTermination
					} else {
						parseStep = stepData
					}
				case stepData:
					commandData[commandPointer] = b
					commandPointer++
					if commandPointer >= commandSize {
						parseStep = stepTermination
					}
				case stepTermination:
					if b == term_nak {
						ca.SetError(fmt.Errorf("%w: %02X", ErrCommandTermination, b))
						parseStep = stepCommand
						continue
					} else if b != term_ack {
						ca.SetError(fmt.Errorf("unexpected termination byte: %02X, expected: %02X", b, term_ack))
						parseStep = stepCommand
						continue
					}
					//commandData[commandPointer] = b
					//commandPointer++
					if command == cmd_brd_adcfilter && commandSize == 1 {
						commandData[3] = 0x01
					}
					if ca.cfg.Debug {
						pkg := []byte{command, byte(commandSize >> 8), byte(commandSize)}
						pkg = append(pkg, commandData[:commandPointer]...)
						log.Printf("recv cmd: % 02X", pkg)
					}
					if err := ca.processCommand(command, commandData[:commandPointer]); err != nil {
						ca.SetError(err)
					}
					parseStep = stepCommand
				}
			}
		}
	}
}

func (ca *CombiAdapter) processCommand(cmd byte, data []byte) error {
	switch cmd {
	case cmd_brd_adcfilter:
		// log.Printf("ADC filter RAW: %X", data)
		switch len(data) {
		case 1:
			if data[0] == 0xFF {
				log.Printf("ADC filter err: %X", data)
			}
		case 2:
			if data[0] == 0x01 {
				ca.adcFilterChan <- true
			} else {
				ca.adcFilterChan <- false
			}
		case 3:
			log.Printf("ADC filter: %X", data)
		default:
			log.Println("invalid ADC filter size", len(data))
		}
		return ca.handleControlCommand(cmd, data)
	case cmd_brd_egt:
		//log.Printf("EGT: %02X, data: %X", cmd, data)
		select {
		case ca.thermoValueChan <- math.Float32frombits(binary.LittleEndian.Uint32(data[1:5])):
		default:
			log.Println("thermoValueChan full, dropping value")
		}
		return ca.handleControlCommand(cmd, data)
	case cmd_brd_adc:
		// log.Printf("ADC: %02X, data: %X", cmd, data)
		select {
		case ca.adcValueChan <- math.Float32frombits(binary.LittleEndian.Uint32(data[0:4])):
		default:
			log.Println("adcValueChan full, dropping value")
		}
		//return ca.handleADC(cmd, data)
		return ca.handleControlCommand(cmd, data)
	case cmd_brd_fwversion, cmd_can_open, cmd_can_bitrate, cmd_can_txframe:
		return ca.handleControlCommand(cmd, data)
	case cmd_can_frame:
		return ca.handleCANFrame(data)
	default:
		return fmt.Errorf("%w: %02X", ErrInvalidCommand, cmd)
	}
}

func (ca *CombiAdapter) handleControlCommand(cmd byte, data []byte) error {
	select {
	case b := <-ca.sendSem:
		if b != cmd {
			return fmt.Errorf("unexpected command: %02X, expected: %02X", cmd, b)
		}
	default:
	}

	return nil
}

func (ca *CombiAdapter) handleCANFrame(data []byte) error {
	if len(data) != 15 {
		return fmt.Errorf("invalid CAN frame size: %d", len(data))
	}
	frame := NewFrame(
		binary.LittleEndian.Uint32(data[:4]),
		data[4:4+data[12]],
		Incoming,
	)
	frame.Extended = data[13] == 1
	frame.RTR = data[14] == 1
	select {
	case ca.recvChan <- frame:
		return nil
	default:
		return ErrDroppedFrame
	}
}

func (ca *CombiAdapter) sendManager(ctx context.Context) {
	if ca.cfg.Debug {
		defer log.Println("sendManager exited")
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-ca.closeChan:
			return
		case frame := <-ca.sendChan:
			if frame.Identifier == SystemMsg {
				ca.sendSem <- frame.Data[0]
				_, err := ca.out.WriteContext(ctx, frame.Data)
				if err != nil {
					ca.SetError(fmt.Errorf("failed to send frame: %w", err))
				}
				continue
			}
			ca.sendCANMessage(ctx, frame)
		}
	}
}

func (ca *CombiAdapter) sendCANMessage(ctx context.Context, frame *CANFrame) {
	buff := ca.txPool.Get().([]byte)
	defer ca.txPool.Put(buff)

	//buff[0] = cmd_can_txframe
	//buff[1] = 0x00 // will never change length in this byte
	//buff[2] = 0x0F
	binary.LittleEndian.PutUint32(buff[3:], frame.Identifier)
	copy(buff[7:], frame.Data[:min(frame.Length(), 8)])

	buff[15] = uint8(frame.Length())

	if frame.Extended {
		buff[16] = 1 // is extended
	} else {
		buff[16] = 0 // not extended
	}
	if frame.RTR {
		buff[17] = 1 // is remote
	} else {
		buff[17] = 0 // not remote
	}
	//buff[18] = 0x00 // terminator

	ca.sendSem <- cmd_can_txframe
	n, err := ca.out.WriteContext(ctx, buff)
	if err != nil {
		ca.SetError(fmt.Errorf("failed to send frame: %w", err))
	}
	if n != 19 {
		ca.SetError(fmt.Errorf("sent %d bytes of data out of 19", n))
	}
}

func (ca *CombiAdapter) canCtrl(mode byte) error {
	ctx, cancel := context.WithTimeout(context.Background(), 400*time.Millisecond)
	defer cancel()
	if _, err := ca.out.WriteContext(ctx, []byte{cmd_can_open, 0x00, 0x01, mode, 0x00}); err != nil {
		return fmt.Errorf("failed to write to usb device: %w", err)
	}
	return nil
}

func (ca *CombiAdapter) setBitrate(ctx context.Context) error {
	var canrate uint32
	if ca.cfg.CANRate == 615.384 {
		canrate = 615000
	} else {
		canrate = uint32(ca.cfg.CANRate * 1000)
	}
	wctx, cancel := context.WithTimeout(ctx, 40*time.Millisecond)
	defer cancel()
	if _, err := ca.out.WriteContext(wctx, []byte{cmd_can_bitrate, 0x00, 0x04, byte(canrate >> 24), byte(canrate >> 16), byte(canrate >> 8), byte(canrate), 0x00}); err != nil {
		return fmt.Errorf("failed to write to usb device: %w", err)
	}
	return nil
}

/*
func frameToPacket(frame CANFrame) *CombiPacket {
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
func (ca *CombiAdapter) sendFrame(ctx context.Context, frame CANFrame) error {
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
				ca.recv <- NewFrame(
					binary.LittleEndian.Uint32(dataBuff[:4]),
					dataBuff[4:4+dataBuff[12]],
					Incoming,
				)
			}

			if ca.cfg.Debug {
				log.Printf("cmd: %02X, len: %d, data: %X, term: %02X", cmd, dataLen, dataBuff[:dataLen-2], dataBuff[dataLen-1])
			}
		}
	}
}
*/

func list() {
	ctx := gousb.NewContext()
	defer ctx.Close()

	// Debugging can be turned on; this shows some of the inner workings of the libusb package.
	ctx.Debug(0)

	// OpenDevices is used to find the devices to open.
	devs, err := ctx.OpenDevices(func(desc *gousb.DeviceDesc) bool {
		// The usbid package can be used to print out human readable information.
		fmt.Printf("%03d.%03d %s:%s %s\n", desc.Bus, desc.Address, desc.Vendor, desc.Product, usbid.Describe(desc))
		fmt.Printf("  Protocol: %s\n", usbid.Classify(desc))

		// The configurations can be examined from the DeviceDesc, though they can only
		// be set once the device is opened.  All configuration references must be closed,
		// to free up the memory in libusb.
		for _, cfg := range desc.Configs {
			// This loop just uses more of the built-in and usbid pretty printing to list
			// the USB devices.
			fmt.Printf("  %s:\n", cfg)
			for _, intf := range cfg.Interfaces {
				fmt.Printf("    --------------\n")
				for _, ifSetting := range intf.AltSettings {
					fmt.Printf("    %s\n", ifSetting)
					fmt.Printf("      %s\n", usbid.Classify(ifSetting))
					for _, end := range ifSetting.Endpoints {
						fmt.Printf("      %s\n", end)
					}
				}
			}
			fmt.Printf("    --------------\n")
		}

		// After inspecting the descriptor, return true or false depending on whether
		// the device is "interesting" or not.  Any descriptor for which true is returned
		// opens a Device which is retuned in a slice (and must be subsequently closed).
		return false
	})

	// All Devices returned from OpenDevices must be closed.
	defer func() {
		for _, d := range devs {
			d.Close()
		}
	}()

	// OpenDevices can occasionally fail, so be sure to check its return value.
	if err != nil {
		log.Fatalf("list: %s", err)
	}

	for _, dev := range devs {
		// Once the device has been selected from OpenDevices, it is opened
		// and can be interacted with.
		_ = dev
	}
}

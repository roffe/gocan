package adapter

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"github.com/google/gousb"
	"github.com/roffe/gocan"
	"runtime"
	"strconv"
	"sync"
)

var wg sync.WaitGroup

func init() {
	if !FindDevice() {
		return
	}
	if err := Register(&AdapterInfo{
		Name:               "CombiAdapter",
		Description:        "LibUsb Driver",
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

type Combi struct {
	cfg        *gocan.AdapterConfig
	send, recv chan gocan.CANFrame
	close      chan struct{}
	usbCtx     *gousb.Context
	dev        *gousb.Device
	epIn       *gousb.InEndpoint
	epOut      *gousb.OutEndpoint
	ucfg       *gousb.Config
	intf       *gousb.Interface
}

type Packet struct {
	cmd  uint8
	len  uint16
	data []uint8
	term uint8
}

const (
	product    = 0x0005
	vendor     = 0xFFFF
	open       = 0x80
	txFrame    = 0x83
	rxFrame    = 0x82
	canBitrate = 0x81
	version    = 0x20
)

func NewCombi(cfg *gocan.AdapterConfig) (gocan.Adapter, error) {
	return &Combi{
		cfg:   cfg,
		send:  make(chan gocan.CANFrame, 1),
		recv:  make(chan gocan.CANFrame, 1),
		close: make(chan struct{}, 1),
	}, nil
}

func (a *Combi) SetFilter(filters []uint32) error {
	return nil
}

func (a *Combi) Name() string {
	return "Combi"
}

func FindDevice() bool {
	ctx := gousb.NewContext()
	defer ctx.Close()
	dev, err := ctx.OpenDeviceWithVIDPID(vendor, product)
	if err != nil || dev == nil {
		return false
	}
	defer dev.Close()
	return true
}

func (a *Combi) Init(ctx context.Context) error {
	a.usbCtx = gousb.NewContext()

	var err error
	a.dev, err = a.usbCtx.OpenDeviceWithVIDPID(vendor, product)
	if err != nil {
		a.cfg.OnError(fmt.Errorf("OpenDevices(): %v", err))
		return err
	}

	err = a.dev.SetAutoDetach(true)
	if err != nil {
		a.cfg.OnError(fmt.Errorf("%s.SetAutoDetach(true): %v", a.dev, err))
	}

	a.ucfg, err = a.dev.Config(1)
	if err != nil {
		a.cfg.OnError(fmt.Errorf("%s.Config(2): %v", a.dev, err))
	}

	a.intf, err = a.ucfg.Interface(1, 0)
	if err != nil {
		a.cfg.OnError(fmt.Errorf("%s.Interface(1, 0): %v", a.ucfg, err))
	}

	a.epIn, err = a.intf.InEndpoint(2)
	if err != nil {
		a.cfg.OnError(fmt.Errorf("%s.InEndpoint(2): %v", a.intf, err))
	}

	a.epOut, err = a.intf.OutEndpoint(5)
	if err != nil {
		a.cfg.OnMessage("EP.OutEndpoint(5):failed, fallback to EP 2 (stm32 clone)")
		a.epOut, err = a.intf.OutEndpoint(2)
		if err != nil {
			return err
		}
	}

	err = a.Open(ctx, 0)
	if err != nil {
		return err
	}

	err, s := a.ReadVersion(ctx)
	if err != nil {
		return err
	}
	a.cfg.OnMessage(s)

	err = a.SetBitrate(ctx)
	if err != nil {
		return err
	}

	err = a.Open(ctx, 1)
	if err != nil {
		return err
	}

	go a.recvManager(ctx)
	go a.sendManager(ctx)
	return nil
}

func (a *Combi) Open(ctx context.Context, mode uint8) error {
	var rx Packet
	tx := Packet{
		cmd:  open,
		len:  1,
		data: []byte{mode},
		term: 0,
	}

	err := a.SendCmd(ctx, tx, &rx)
	if err != nil {
		return err
	}
	if rx.cmd != tx.cmd || rx.term != 0 {
		return errors.New("woops")
	}
	return nil
}

func (a *Combi) ReadVersion(ctx context.Context) (error, string) {
	var rx Packet
	var ver string
	tx := Packet{
		cmd:  version,
		len:  0,
		data: nil,
		term: 0,
	}

	err := a.SendCmd(ctx, tx, &rx)
	if err != nil {
		return err, ver
	}
	if rx.cmd != tx.cmd || rx.term != 0 {
		return errors.New("woops"), ver
	}
	ver = "CombiAdapter: v" + strconv.Itoa(int(rx.data[1])) + "." + strconv.Itoa(int(rx.data[0]))
	return nil, ver
}

func (a *Combi) SetBitrate(ctx context.Context) error {
	var rx Packet
	baudBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(baudBuf, uint32(a.cfg.CANRate*1000))
	tx := Packet{
		cmd:  canBitrate,
		len:  4,
		data: baudBuf,
		term: 0,
	}

	err := a.SendCmd(ctx, tx, &rx)
	if err != nil {
		return err
	}

	if rx.cmd != tx.cmd || rx.term != 0 {
		return errors.New("woops")
	}
	return nil
}

func (a *Combi) txFrame(ctx context.Context, f gocan.CANFrame) error {
	wg.Add(1)
	buff := make([]byte, 15)
	binary.LittleEndian.PutUint32(buff, f.Identifier())
	copy(buff[4:], f.Data())
	buff[12] = uint8(f.Length())
	buff[13] = 0
	buff[14] = 0

	tx := Packet{
		cmd:  txFrame,
		len:  15,
		data: buff,
		term: 0,
	}

	err := a.SendCmd(ctx, tx, nil)
	if err != nil {
		return err
	}

	wg.Wait()

	return err
}

func (a *Combi) SendCmd(ctx context.Context, tx Packet, rx *Packet) error {
	buf := &bytes.Buffer{}
	buf.WriteByte(tx.cmd)
	buf.WriteByte(uint8(tx.len >> 8))
	buf.WriteByte(uint8(tx.len & 0xff))
	if tx.data != nil {
		buf.Write(tx.data)
	}
	buf.WriteByte(tx.term)
	writeBytes, err := a.epOut.WriteContext(ctx, buf.Bytes())
	if err != nil {
		fmt.Println("Write returned an error:", err)
	}
	if writeBytes != buf.Len() {
		a.cfg.OnError(fmt.Errorf("sent %d bytes of data out of %d", writeBytes, buf.Len()))
	}
	if rx == nil {
		return err
	}
	err = a.readPacket(ctx, rx)
	return err
}

func (a *Combi) readPacket(ctx context.Context, rx *Packet) error {
	rBuf := make([]byte, 256)
	readBytes, err := a.epIn.ReadContext(ctx, rBuf)
	if err != nil {
		fmt.Println("Read returned an error:", err)
	}
	if readBytes == 0 {
		a.cfg.OnError(fmt.Errorf("received 0 bytes of data"))
		return err
	}
	rx.cmd = rBuf[0]
	rx.len = uint16(rBuf[1])<<8 | uint16(rBuf[2])
	if rx.len > 0 {
		rx.data = rBuf[3 : rx.len+3]
	}
	rx.term = rBuf[rx.len+3]
	return err
}

func (a *Combi) recvManager(ctx context.Context) {
	runtime.LockOSThread()
	for {
		select {
		case <-a.close:
			return
		default:
			var rx Packet
			err := a.readPacket(ctx, &rx)
			if err != nil {
				continue
			}
			switch rx.cmd {
			case rxFrame:
				if rx.len != 15 {
					err = errors.New("woops")
					return
				}
				frame := gocan.NewFrame(
					binary.LittleEndian.Uint32(rx.data[:4]),
					rx.data[4:rx.data[12]+4],
					gocan.Incoming,
				)
				select {
					case a.recv <- frame
					default:
						a.cfg.OnError(ErrDroppedFrame)
				}
				break
			case txFrame:
				wg.Done()
				break
			}
		}
	}
}

func (a *Combi) sendManager(ctx context.Context) {
	runtime.LockOSThread()
	for {
		select {
		case <-a.close:
			return
		case <-ctx.Done():
			return
		case f := <-a.send:
			err := a.txFrame(ctx, f)
			if err != nil {
				a.cfg.OnError(fmt.Errorf("send error: %w", err))
			}
		}
	}
}

func (a *Combi) Recv() <-chan gocan.CANFrame {
	return a.recv
}

func (a *Combi) Send() chan<- gocan.CANFrame {
	return a.send
}

func (a *Combi) Close() error {
	close(a.close)
	a.intf.Close()
	a.dev.Close()
	a.usbCtx.Close()
	a.ucfg.Close()
	return nil
}

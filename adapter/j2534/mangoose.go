package j2534

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"syscall"
	"time"
	"unsafe"

	"github.com/roffe/gocan"
)

type Mangoose struct {
	h                                    *J2534PassThru
	channelID, deviceID, flags, protocol uint32
	cfg                                  *gocan.AdapterConfig
	send, recv                           chan gocan.CANFrame
	close                                chan struct{}

	*syscall.LazyProc
}

func NewMangoose(cfg *gocan.AdapterConfig) (gocan.Adapter, error) {
	ma := &Mangoose{
		cfg:   cfg,
		send:  make(chan gocan.CANFrame, 100),
		recv:  make(chan gocan.CANFrame, 100),
		close: make(chan struct{}, 1),
	}
	return ma, nil
}

func (ma *Mangoose) Init(ctx context.Context) error {
	ma.h = NewJ2534("C:\\Program Files (x86)\\Drew Technologies, Inc.\\J2534\\MongoosePro GM II\\monpa432.dll")

	if err := ma.h.PassThruOpen("", &ma.deviceID); err != nil {
		return err
	}
	ma.printVersions()
	var baudRate uint32
	switch ma.cfg.CANRate {
	case 33.3:
		baudRate = 33300
		ma.protocol = SW_CAN_PS
	case 500:
		baudRate = 500000
		ma.protocol = CAN
	default:
		return errors.New("invalid CAN rate")
	}

	if err := ma.h.PassThruConnect(ma.deviceID, ma.protocol, ma.flags, baudRate, &ma.channelID); err != nil {
		return err
	}

	if ma.protocol == SW_CAN_PS {
		input1 := &SCONFIG_LIST{
			NumOfParams: 1,
			ConfigPtr: &SCONFIG{
				Parameter: J1962_PINS,
				Value:     0x0100,
			},
		}

		if err := ma.h.PassThruIoctl(ma.channelID, SET_CONFIG, (*byte)(unsafe.Pointer(input1)), nil); err != nil {
			str, err2 := ma.h.PassThruGetLastError()
			if err2 != nil {
				log.Println(err2)
			} else {
				log.Println(str)
			}
			return err
		}
	}

	if err := ma.h.PassThruIoctl(ma.channelID, CLEAR_TX_BUFFER, nil, nil); err != nil {
		return err
	}

	if err := ma.h.PassThruIoctl(ma.channelID, CLEAR_RX_BUFFER, nil, nil); err != nil {
		return err
	}

	if len(ma.cfg.CANFilter) > 0 {
		ma.setupFilters()
	} else {
		ma.allowAll()
	}

	time.Sleep(10 * time.Millisecond)

	go ma.recvManager()
	go ma.sendManager()

	return nil
}

func (ma *Mangoose) allowAll() {
	filterID := uint32(0)
	var MaskMsg, PatternMsg PASSTHRU_MSG

	mask := [4]byte{0x00, 0x00, 0x00, 0x00}
	MaskMsg.ProtocolID = ma.protocol
	copy(MaskMsg.Data[:], mask[:])
	MaskMsg.DataSize = 4

	pattern := [4]byte{0x00, 0x00, 0x00, 0x00}
	PatternMsg.ProtocolID = ma.protocol
	copy(PatternMsg.Data[:], pattern[:])
	PatternMsg.DataSize = 4

	if err := ma.h.PassThruStartMsgFilter(ma.channelID, PASS_FILTER, &MaskMsg, &PatternMsg, nil, &filterID); err != nil {
		log.Fatal(err)
	}
}

func (ma *Mangoose) setupFilters() error {
	if len(ma.cfg.CANFilter) > 10 {
		return errors.New("too many filters")
	}

	var MaskMsg, PatternMsg PASSTHRU_MSG
	mask := [4]byte{0xFF, 0xFF, 0xFF, 0xFF}
	MaskMsg.ProtocolID = ma.protocol
	copy(MaskMsg.Data[:], mask[:])
	MaskMsg.DataSize = 4

	for i, filter := range ma.cfg.CANFilter {
		filterID := new(uint32)
		*filterID = uint32(i)
		var pattern = make([]byte, 4)
		binary.BigEndian.PutUint32(pattern, filter)
		PatternMsg.ProtocolID = ma.protocol
		if n := copy(PatternMsg.Data[:], pattern); n != len(pattern) {
			return errors.New("copy failed to pattern data")
		}
		PatternMsg.DataSize = 4
		log.Printf("filter: %08X", filter)
		if err := ma.h.PassThruStartMsgFilter(ma.channelID, PASS_FILTER, &MaskMsg, &PatternMsg, nil, filterID); err != nil {
			return err
		}
	}

	return nil
}

func (ma *Mangoose) recvManager() {
outer:
	for {
		select {
		case <-ma.close:
			log.Println("closing recv manager")
			return
		default:
			msg := new(PASSTHRU_MSG)
			msg.ProtocolID = ma.protocol
			if err := ma.h.PassThruReadMsgs(ma.channelID, uintptr(unsafe.Pointer(msg)), 1, 1500); err != nil {
				if errors.Is(err, ErrBufferEmpty) {
					continue
				}
				if errors.Is(err, ErrDeviceNotConnected) {
					break outer
				}

				log.Println("read", err)
				continue
			}
			var id uint32

			if err := binary.Read(bytes.NewReader(msg.Data[:]), binary.BigEndian, &id); err != nil {
				log.Println("read", err)
				continue
			}
			f := gocan.NewFrame(id, msg.Data[4:msg.DataSize], gocan.Incoming)
			ma.recv <- f
		}
	}
}

func (ma *Mangoose) sendManager() {
	for {
		select {
		case <-ma.close:
			return
		case f := <-ma.send:
			var buf bytes.Buffer
			binary.Write(&buf, binary.BigEndian, f.Identifier())
			buf.Write(f.Data())
			msg := &PASSTHRU_MSG{
				ProtocolID: ma.protocol,
				DataSize:   uint32(buf.Len()),
			}
			if ma.protocol == SW_CAN_PS {
				msg.TxFlags = SW_CAN_HV_TX
			}

			copy(msg.Data[:], buf.Bytes())
			if err := ma.h.PassThruWriteMsgs(ma.channelID, uintptr(unsafe.Pointer(msg)), 1, 1500); err != nil {
				log.Println("send:", err)
			}
		}
	}
}

func (ma *Mangoose) Recv() <-chan gocan.CANFrame {
	return ma.recv
}

func (ma *Mangoose) Send() chan<- gocan.CANFrame {
	return ma.send
}

func (ma *Mangoose) Close() error {
	close(ma.close)
	time.Sleep(100 * time.Millisecond)

	if err := ma.h.PassThruIoctl(ma.channelID, CLEAR_MSG_FILTERS, nil, nil); err != nil {
		return err
	}

	if err := ma.h.PassThruDisconnect(ma.channelID); err != nil {
		return err
	}
	log.Println("disconnected")

	return ma.h.PassThruClose(ma.deviceID)
}

func (ma *Mangoose) printVersions() {
	firmwareVersion, dllVersion, apiVersion, err := ma.h.PassThruReadVersion(ma.deviceID)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Firmware version:", firmwareVersion)
	fmt.Println("DLL version:", dllVersion)
	fmt.Println("API version:", apiVersion)
}

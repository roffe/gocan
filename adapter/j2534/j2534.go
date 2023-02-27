package j2534

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"runtime"
	"syscall"
	"time"
	"unsafe"

	"github.com/roffe/gocan"
)

type J2534 struct {
	h                                    *PassThru
	channelID, deviceID, flags, protocol uint32
	cfg                                  *gocan.AdapterConfig
	send, recv                           chan gocan.CANFrame
	close                                chan struct{}

	*syscall.LazyProc
}

func New(cfg *gocan.AdapterConfig) (gocan.Adapter, error) {
	ma := &J2534{
		cfg:   cfg,
		send:  make(chan gocan.CANFrame, 10),
		recv:  make(chan gocan.CANFrame, 10),
		close: make(chan struct{}, 1),
	}
	return ma, nil
}

func (ma *J2534) Name() string {
	return "J2534"
}

func (ma *J2534) output(str string) {
	if ma.cfg.Output != nil {
		ma.cfg.Output(str)
	} else {
		log.Println(str)
	}
}

func (ma *J2534) Init(ctx context.Context) error {
	var err error
	ma.h, err = NewJ2534(ma.cfg.Port)
	if err != nil {
		return err
	}

	var swcan bool
	var baudRate uint32
	switch ma.cfg.CANRate {
	case 33.3:
		baudRate = 33300
		ma.protocol = SW_CAN_PS
		swcan = true
	case 500:
		baudRate = 500000
		ma.protocol = CAN
	case 615.384:
		baudRate = 615384
		ma.protocol = CAN
	default:
		return errors.New("invalid CAN rate")
	}

	if err := ma.h.PassThruOpen("", &ma.deviceID); err != nil {
		str, err2 := ma.h.PassThruGetLastError()
		if err2 != nil {
			ma.output(err2.Error())
		} else {
			ma.output("LR" + str)
		}
		ma.Close()
		return fmt.Errorf("PassThruOpen: %w", err)
	}

	//if err := ma.printVersions(); err != nil {
	//	return err
	//}

	if err := ma.h.PassThruConnect(ma.deviceID, ma.protocol, 0x1000000, baudRate, &ma.channelID); err != nil {
		ma.Close()
		return fmt.Errorf("PassThruConnect: %w", err)
	}

	if swcan {
		input1 := &SCONFIG_LIST{
			NumOfParams: 1,
			ConfigPtr: &SCONFIG{
				Parameter: J1962_PINS,
				Value:     0x0100,
			},
		}

		if err := ma.h.PassThruIoctl(ma.channelID, SET_CONFIG, (*byte)(unsafe.Pointer(input1)), nil); err != nil {
			ma.Close()
			return fmt.Errorf("PassThruIoctl set SWCAN: %w", err)
		}
	}

	if err := ma.h.PassThruIoctl(ma.channelID, CLEAR_TX_BUFFER, nil, nil); err != nil {
		ma.Close()
		return fmt.Errorf("PassThruIoctl clear tx buffer: %w", err)
	}

	if err := ma.h.PassThruIoctl(ma.channelID, CLEAR_RX_BUFFER, nil, nil); err != nil {
		ma.Close()
		return fmt.Errorf("PassThruIoctl clear rx buffer: %w", err)
	}

	if len(ma.cfg.CANFilter) > 0 {
		ma.setupFilters()
	} else {
		ma.allowAll()
	}

	go ma.recvManager()
	go ma.sendManager()

	return nil
}

func (ma *J2534) allowAll() {
	filterID := uint32(0)
	var MaskMsg, PatternMsg PassThruMsg
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

func (ma *J2534) setupFilters() error {
	if len(ma.cfg.CANFilter) > 10 {
		return errors.New("too many filters")
	}

	var MaskMsg, PatternMsg PassThruMsg
	mask := [4]byte{0xff, 0xff, 0xff, 0xff}
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
			ma.Close()
			return errors.New("copy failed to pattern data")
		}
		PatternMsg.DataSize = 4
		if err := ma.h.PassThruStartMsgFilter(ma.channelID, PASS_FILTER, &MaskMsg, &PatternMsg, nil, filterID); err != nil {
			ma.Close()
			return err
		}
	}

	return nil
}

func (ma *J2534) recvManager() {
	runtime.LockOSThread()
	for {
		select {
		case <-ma.close:
			return
		default:
			msg := new(PassThruMsg)
			msg.ProtocolID = ma.protocol
			if err := ma.h.PassThruReadMsgs(ma.channelID, uintptr(unsafe.Pointer(msg)), 1, 0); err != nil {
				if errors.Is(err, ErrBufferEmpty) {
					continue
				}
				if errors.Is(err, ErrDeviceNotConnected) {
					return
				}

				ma.output("read error: " + err.Error())
				continue
			}
			var id uint32

			if err := binary.Read(bytes.NewReader(msg.Data[:]), binary.BigEndian, &id); err != nil {
				ma.output("read CAN ID error: " + err.Error())
				continue
			}
			f := gocan.NewFrame(id, msg.Data[4:msg.DataSize], gocan.Incoming)
			ma.recv <- f
		}
	}
}

func (ma *J2534) sendManager() {
	var buf bytes.Buffer
	for {
		select {
		case <-ma.close:
			return
		case f := <-ma.send:
			buf.Reset()
			if err := binary.Write(&buf, binary.BigEndian, f.Identifier()); err != nil {
				ma.output("unable to write CAN ID to buffer: " + err.Error())
				continue
			}
			buf.Write(f.Data())
			msg := &PassThruMsg{
				ProtocolID: ma.protocol,
				DataSize:   uint32(buf.Len()),
				TxFlags:    0,
			}
			if ma.protocol == SW_CAN_PS {
				msg.TxFlags = SW_CAN_HV_TX
			}
			copy(msg.Data[:], buf.Bytes())
			if err := ma.h.PassThruWriteMsgs(ma.channelID, uintptr(unsafe.Pointer(msg)), 1, 0); err != nil {
				ma.output("send error: " + err.Error())
			}
		}
	}
}

func (ma *J2534) Recv() <-chan gocan.CANFrame {
	return ma.recv
}

func (ma *J2534) Send() chan<- gocan.CANFrame {
	return ma.send
}

func (ma *J2534) Close() error {
	close(ma.close)
	time.Sleep(200 * time.Millisecond)
	ma.h.PassThruIoctl(ma.channelID, CLEAR_MSG_FILTERS, nil, nil)
	ma.h.PassThruDisconnect(ma.channelID)
	ma.h.PassThruClose(ma.deviceID)
	return ma.h.Close()
}

func (ma *J2534) printVersions() error {
	firmwareVersion, dllVersion, apiVersion, err := ma.h.PassThruReadVersion(ma.deviceID)
	if err != nil {
		return err
	}
	if ma.cfg.Output != nil {
		ma.cfg.Output(fmt.Sprintf("Firmware version: %s", firmwareVersion))
		ma.cfg.Output(fmt.Sprintf("DLL version: %s", dllVersion))
		ma.cfg.Output(fmt.Sprintf("API version: %s", apiVersion))

	} else {
		fmt.Println("Firmware version:", firmwareVersion)
		fmt.Println("DLL version:", dllVersion)
		fmt.Println("API version:", apiVersion)
	}
	return nil
}

package j2534

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"runtime"
	"strings"
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
		cfg:       cfg,
		send:      make(chan gocan.CANFrame, 10),
		recv:      make(chan gocan.CANFrame, 10),
		close:     make(chan struct{}, 1),
		channelID: 1,
		deviceID:  1,
	}
	return ma, nil
}

func (ma *J2534) Name() string {
	return "J2534"
}

func (ma *J2534) Init(ctx context.Context) error {
	var err error
	ma.h, err = NewJ2534(ma.cfg.Port)
	if err != nil {
		return err
	}
	var t2 bool
	if strings.HasSuffix(ma.cfg.Port, "Tech2_32.dll") {
		t2 = true
	}

	var swcan bool
	var baudRate uint32
	switch ma.cfg.CANRate {
	case 33.3:
		baudRate = 33333
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
			ma.cfg.ErrorFunc(fmt.Errorf("PassThruOpenGetLastError: %w", err))
		} else {
			ma.cfg.OutputFunc("PassThruOpen: " + str)
		}
		return fmt.Errorf("PassThruOpen: %w", err)
	}

	//if err := ma.printVersions(); err != nil {
	//	return err
	//}

	if t2 {
		time.Sleep(1 * time.Second)
	}

	//ma.cfg.OutputFunc(fmt.Sprintf("PassThruConnect: %d %X %X %d", ma.deviceID, ma.protocol, ma.flags, baudRate))
	if err := ma.h.PassThruConnect(ma.deviceID, ma.protocol, ma.flags, baudRate, &ma.channelID); err != nil {
		return fmt.Errorf("PassThruConnect: %w", err)
	}

	if swcan {
		opts := &SCONFIG_LIST{
			NumOfParams: 1,
			ConfigPtr: &SCONFIG{
				Parameter: J1962_PINS,
				Value:     0x0100,
			},
		}
		if err := ma.h.PassThruIoctl(ma.channelID, SET_CONFIG, (*byte)(unsafe.Pointer(opts)), nil); err != nil {
			return fmt.Errorf("PassThruIoctl set SWCAN: %w", err)
		}
		if t2 {
			time.Sleep(1 * time.Second)
		}
	}

	if t2 {
		opts := &SCONFIG_LIST{
			NumOfParams: 1,
			ConfigPtr: &SCONFIG{
				Parameter: LOOPBACK,
				Value:     0x1,
			},
		}
		if err := ma.h.PassThruIoctl(ma.channelID, SET_CONFIG, (*byte)(unsafe.Pointer(opts)), nil); err != nil {
			return fmt.Errorf("PassThruIoctl set SWCAN: %w", err)
		}
		time.Sleep(1 * time.Second)
	}

	if err := ma.h.PassThruIoctl(ma.channelID, CLEAR_RX_BUFFER, nil, nil); err != nil {
		return fmt.Errorf("PassThruIoctl clear rx buffer: %w", err)
	}

	if len(ma.cfg.CANFilter) > 0 {
		ma.setupFilters()
	} else {
		ma.allowAll()
	}
	if t2 {
		time.Sleep(1 * time.Second)
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
		ma.cfg.ErrorFunc(fmt.Errorf("PassThruStartMsgFilter: %w", err))
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
			return errors.New("copy failed to pattern data")
		}
		PatternMsg.DataSize = 4
		if err := ma.h.PassThruStartMsgFilter(ma.channelID, PASS_FILTER, &MaskMsg, &PatternMsg, nil, filterID); err != nil {
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
				ma.cfg.ErrorFunc(fmt.Errorf("read error: %w", err))
				continue
			}
			var id uint32

			if err := binary.Read(bytes.NewReader(msg.Data[:]), binary.BigEndian, &id); err != nil {
				ma.cfg.ErrorFunc(fmt.Errorf("read CAN ID error: %w", err))
				continue
			}
			if msg.DataSize == 0 {
				ma.cfg.ErrorFunc(fmt.Errorf("empty message: %d", id))
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
				ma.cfg.ErrorFunc(fmt.Errorf("unable to write CAN ID to buffer:  %w", err))
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
				ma.cfg.ErrorFunc(fmt.Errorf("send error: %w", err))
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
	ma.cfg.OutputFunc(fmt.Sprintf("Firmware version: %s", firmwareVersion))
	ma.cfg.OutputFunc(fmt.Sprintf("DLL version: %s", dllVersion))
	ma.cfg.OutputFunc(fmt.Sprintf("API version: %s", apiVersion))
	return nil
}

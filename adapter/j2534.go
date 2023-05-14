package adapter

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/roffe/gocan"
	"github.com/roffe/gocan/adapter/passthru"
)

func init() {
	for _, dll := range passthru.FindDLLs() {
		if err := Register(&AdapterInfo{
			Name:               dll.Name,
			Description:        "J2534 Interface",
			RequiresSerialPort: false,
			Capabilities: AdapterCapabilities{
				HSCAN: dll.Capabilities.CAN || dll.Capabilities.CANPS,
				KLine: dll.Capabilities.ISO9141 || dll.Capabilities.ISO14230,
				SWCAN: dll.Capabilities.SWCANPS,
			},
			New: NewJ2534FromDLLName(dll.FunctionLibrary),
		}); err != nil {
			panic(err)
		}
	}
}

type J2534 struct {
	h                                    *passthru.PassThru
	channelID, deviceID, flags, protocol uint32
	cfg                                  *gocan.AdapterConfig
	send, recv                           chan gocan.CANFrame
	close                                chan struct{}

	tech2passThru bool
	sync.Mutex
}

func NewJ2534FromDLLName(dllPath string) func(cfg *gocan.AdapterConfig) (gocan.Adapter, error) {
	return func(cfg *gocan.AdapterConfig) (gocan.Adapter, error) {
		cfg.Port = dllPath
		return NewJ2534(cfg)
	}
}

func NewJ2534(cfg *gocan.AdapterConfig) (gocan.Adapter, error) {
	ma := &J2534{
		cfg:       cfg,
		send:      make(chan gocan.CANFrame, 10),
		recv:      make(chan gocan.CANFrame, 20),
		close:     make(chan struct{}, 2),
		channelID: 1,
		deviceID:  1,
	}
	return ma, nil
}

func (ma *J2534) SetFilter(filters []uint32) error {
	if err := ma.h.PassThruClearMsgFilters(ma.channelID); err != nil {
		return err
	}
	ma.cfg.CANFilter = filters
	if len(ma.cfg.CANFilter) > 0 {
		if err := ma.setupFilters(); err != nil {
			return err
		}
	} else {
		ma.allowAll()
	}
	return nil
}

func (ma *J2534) Name() string {
	return "J2534"
}

func (ma *J2534) Init(ctx context.Context) error {
	var err error
	ma.h, err = passthru.New(ma.cfg.Port)
	if err != nil {
		return err
	}

	if strings.HasSuffix(ma.cfg.Port, "Tech2_32.dll") {
		ma.tech2passThru = true
	}

	var swcan bool
	var baudRate uint32
	switch ma.cfg.CANRate {
	case 33.3:
		baudRate = 33333
		ma.protocol = passthru.SW_CAN_PS
		swcan = true
	case 47.619:
		baudRate = 47619
		ma.protocol = passthru.CAN
	case 500:
		baudRate = 500000
		ma.protocol = passthru.CAN
	case 615.384:
		baudRate = 615384
		ma.protocol = passthru.CAN
	default:
		return errors.New("invalid CAN rate")
	}

	if err := ma.h.PassThruOpen("", &ma.deviceID); err != nil {
		str, err2 := ma.h.PassThruGetLastError()
		if err2 != nil {
			ma.cfg.OnError(fmt.Errorf("PassThruOpenGetLastError: %w", err))
		} else {
			ma.cfg.OnMessage("PassThruOpen: " + str)
		}
		if errg := ma.h.Close(); errg != nil {
			ma.cfg.OnError(errg)
		}
		return fmt.Errorf("PassThruOpen: %w", err)
	}

	if ma.cfg.PrintVersion {
		if err := ma.PrintVersions(); err != nil {
			return fmt.Errorf("PassThruPrintVersion: %w", err)
		}
	}

	if err := ma.h.PassThruConnect(ma.deviceID, ma.protocol, ma.flags, baudRate, &ma.channelID); err != nil {
		if errg := ma.h.Close(); errg != nil {
			ma.cfg.OnError(errg)
		}
		return fmt.Errorf("PassThruConnect: %w", err)
	}

	if swcan {
		opts := &passthru.SCONFIG_LIST{
			NumOfParams: 1,
			Params: []passthru.SCONFIG{
				{
					Parameter: passthru.J1962_PINS,
					Value:     0x0100,
				},
			},
		}
		if err := ma.h.PassThruIoctl(ma.channelID, passthru.SET_CONFIG, opts, nil); err != nil {
			if errg := ma.h.Close(); errg != nil {
				ma.cfg.OnError(errg)
			}
			return fmt.Errorf("PassThruIoctl set SWCAN: %w", err)
		}
	}

	if err := ma.h.PassThruIoctl(ma.channelID, passthru.CLEAR_RX_BUFFER, nil, nil); err != nil {
		if errg := ma.h.Close(); errg != nil {
			ma.cfg.OnError(errg)
		}
		return fmt.Errorf("PassThruIoctl clear rx buffer: %w", err)
	}

	if len(ma.cfg.CANFilter) > 0 {
		err := ma.setupFilters()
		if err != nil {
			return err
		}
	} else {
		ma.allowAll()
	}
	go ma.recvManager()
	go ma.sendManager()

	return nil
}

func (ma *J2534) PrintVersions() error {
	firmwareVersion, dllVersion, apiVersion, err := ma.h.PassThruReadVersion(ma.deviceID)
	if err != nil {
		return err
	}
	ma.cfg.OnMessage("Firmware version: " + firmwareVersion)
	ma.cfg.OnMessage("DLL version: " + dllVersion)
	ma.cfg.OnMessage("API version: " + apiVersion)
	return nil
}

func (ma *J2534) allowAll() {
	filterID := uint32(0)
	maskMsg := &passthru.PassThruMsg{
		ProtocolID:     ma.protocol,
		DataSize:       4,
		ExtraDataIndex: 4,
		Data:           [4128]byte{0x00, 0x00, 0x00, 0x00},
	}
	patternMsg := &passthru.PassThruMsg{
		ProtocolID:     ma.protocol,
		DataSize:       4,
		ExtraDataIndex: 4,
		Data:           [4128]byte{0x00, 0x00, 0x00, 0x00},
	}
	if err := ma.h.PassThruStartMsgFilter(ma.channelID, passthru.PASS_FILTER, maskMsg, patternMsg, nil, &filterID); err != nil {
		ma.cfg.OnError(fmt.Errorf("PassThruStartMsgFilter: %w", err))
	}
}

func (ma *J2534) setupFilters() error {
	if len(ma.cfg.CANFilter) > 10 {
		return errors.New("too many filters")
	}
	maskMsg := &passthru.PassThruMsg{
		ProtocolID:     ma.protocol,
		DataSize:       4,
		ExtraDataIndex: 4,
		Data:           [4128]byte{0x00, 0x00, 0xff, 0xff},
	}
	for i, filter := range ma.cfg.CANFilter {
		filterID := uint32(i)
		patternMsg := &passthru.PassThruMsg{
			ProtocolID:     ma.protocol,
			DataSize:       4,
			ExtraDataIndex: 4,
		}
		binary.BigEndian.PutUint32(patternMsg.Data[:], filter)
		if err := ma.h.PassThruStartMsgFilter(ma.channelID, passthru.PASS_FILTER, maskMsg, patternMsg, nil, &filterID); err != nil {
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
			msg, err := ma.readMsg()
			if err != nil {
				ma.cfg.OnError(err)
				continue
			}
			if msg == nil {
				continue
			}
			if msg.DataSize == 0 {
				ma.cfg.OnError(errors.New("empty message"))
				continue
			}
			ma.recv <- gocan.NewFrame(
				binary.BigEndian.Uint32(msg.Data[0:4]),
				msg.Data[4:msg.DataSize],
				gocan.Incoming,
			)
		}
	}
}

func (ma *J2534) readMsg() (*passthru.PassThruMsg, error) {
	if ma.tech2passThru {
		ma.Lock()
		defer ma.Unlock()
	}
	msg := &passthru.PassThruMsg{
		ProtocolID: ma.protocol,
	}
	numMsgs := uint32(1)
	if err := ma.h.PassThruReadMsgs(ma.channelID, msg, &numMsgs, 10); err != nil {
		if errors.Is(err, passthru.ErrBufferEmpty) {
			return nil, nil
		}
		if errors.Is(err, passthru.ErrDeviceNotConnected) {
			return nil, fmt.Errorf("device not connected: %w", err)
		}
		return nil, fmt.Errorf("read error: %w", err)
	}
	return msg, nil
}

func (ma *J2534) sendManager() {
	runtime.LockOSThread()
	for {
		select {
		case <-ma.close:
			return
		case f := <-ma.send:
			msg := &passthru.PassThruMsg{
				ProtocolID:     ma.protocol,
				DataSize:       uint32(f.Length() + 4),
				ExtraDataIndex: uint32(f.Length() + 4),
				TxFlags:        0,
			}
			if ma.protocol == passthru.SW_CAN_PS && !ma.tech2passThru {
				msg.TxFlags = passthru.SW_CAN_HV_TX
			}
			binary.BigEndian.PutUint32(msg.Data[:], f.Identifier())
			copy(msg.Data[4:], f.Data())
			if err := ma.sendMsg(msg); err != nil {
				ma.cfg.OnError(fmt.Errorf("send error: %w", err))
			}
		}
	}
}

func (ma *J2534) sendMsg(msg *passthru.PassThruMsg) error {
	if ma.tech2passThru {
		ma.Lock()
		defer ma.Unlock()
	}
	numMsg := uint32(1)
	if err := ma.h.PassThruWriteMsgs(ma.channelID, msg, &numMsg, 100); err != nil {
		if errStr, err2 := ma.h.PassThruGetLastError(); err2 == nil {
			return fmt.Errorf("%w: %s", err, errStr)
		}
		return err
	}
	return nil
}

func (ma *J2534) Recv() <-chan gocan.CANFrame {
	return ma.recv
}

func (ma *J2534) Send() chan<- gocan.CANFrame {
	return ma.send
}

func (ma *J2534) Close() error {
	for i := 0; i < 2; i++ {
		ma.close <- struct{}{}
	}
	time.Sleep(200 * time.Millisecond)
	err := ma.h.PassThruIoctl(ma.channelID, passthru.CLEAR_MSG_FILTERS, nil, nil)
	if err != nil {
		return err
	}
	err = ma.h.PassThruDisconnect(ma.channelID)
	if err != nil {
		return err
	}
	err = ma.h.PassThruClose(ma.deviceID)
	if err != nil {
		return err
	}
	return ma.h.Close()
}

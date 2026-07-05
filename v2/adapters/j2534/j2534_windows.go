//go:build j2534

package j2534

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	gocan "github.com/roffe/gocan/v2"
	"github.com/roffe/gocan/v2/pkg/passthru"
)

func init() {
	prefix, dlls := passthru.FindDLLs()
	for i, dll := range dlls {
		name := fmt.Sprintf("%sJ2534 #%d %s", prefix, i, dll.Name)
		dllPath := dll.FunctionLibrary
		gocan.Register(gocan.AdapterInfo{
			Name:        name,
			Description: "J2534 Interface",
			Capabilities: gocan.Capabilities{
				HSCAN: dll.Capabilities.CAN || dll.Capabilities.CANPS,
				KLine: dll.Capabilities.ISO9141 || dll.Capabilities.ISO14230,
				SWCAN: dll.Capabilities.SWCANPS,
			},
			New: func(cfg gocan.Config) (gocan.Adapter, error) {
				cfg.Port = dllPath
				return New(cfg)
			},
		})
	}
}

type J2534 struct {
	cfg gocan.Config
	bus *gocan.Bus

	h *passthru.PassThru

	channelID uint32
	deviceID  uint32
	flags     uint32
	protocol  uint32

	// Tech2_32.dll is not thread-safe: serialize reads against writes.
	tech2passThru bool
	mu            sync.Mutex

	filters []uint32
}

func New(cfg gocan.Config) (gocan.Adapter, error) {
	return &J2534{
		cfg:     cfg,
		flags:   passthru.CAN_ID_BOTH | passthru.CAN_29BIT_ID,
		filters: cfg.CANFilter,
	}, nil
}

func (ma *J2534) Open(ctx context.Context, bus *gocan.Bus) error {
	ma.bus = bus
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
	case 250:
		baudRate = 250000
		ma.protocol = passthru.CAN
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
		ma.h.Close()
		return errors.New("invalid CAN rate")
	}

	if err := ma.h.PassThruOpen("", &ma.deviceID); err != nil {
		if str, err2 := ma.h.PassThruGetLastError(); err2 == nil && str != "" {
			ma.emit(gocan.EventTypeInfo, "PassThruOpen: "+str)
		}
		ma.h.Close()
		return fmt.Errorf("PassThruOpen: %w", err)
	}

	if firmwareVersion, dllVersion, apiVersion, err := ma.h.PassThruReadVersion(ma.deviceID); err == nil {
		ma.emit(gocan.EventTypeInfo, fmt.Sprintf("Firmware: %s DLL: %s API: %s", firmwareVersion, dllVersion, apiVersion))
	}

	if err := ma.h.PassThruConnect(ma.deviceID, ma.protocol, ma.flags, baudRate, &ma.channelID); err != nil {
		ma.h.Close()
		return fmt.Errorf("PassThruConnect: %w", err)
	}

	if ma.tech2passThru {
		time.Sleep(2 * time.Second)
	}

	if swcan {
		opts := &passthru.SCONFIG_LIST{
			NumOfParams: 1,
			Params: []passthru.SCONFIG{
				{Parameter: passthru.J1962_PINS, Value: 0x0100},
			},
		}
		if err := ma.h.PassThruIoctl(ma.channelID, passthru.SET_CONFIG, opts, nil); err != nil {
			if st, err2 := ma.h.PassThruGetLastError(); err2 == nil && st != "" {
				ma.emit(gocan.EventTypeError, st)
			}
			ma.h.Close()
			return fmt.Errorf("PassThruIoctl set SWCAN: %w", err)
		}
		time.Sleep(100 * time.Millisecond)
	}

	if err := ma.h.PassThruIoctl(ma.channelID, passthru.CLEAR_RX_BUFFER, nil, nil); err != nil {
		ma.h.Close()
		return fmt.Errorf("PassThruIoctl clear rx buffer: %w", err)
	}

	if len(ma.filters) > 0 {
		if err := ma.setupFilters(); err != nil {
			return err
		}
	} else {
		ma.allowAll()
	}

	go ma.readLoop(ctx)
	return nil
}

func (ma *J2534) Close() error {
	time.Sleep(200 * time.Millisecond) // let in-flight frames drain
	if err := ma.h.PassThruIoctl(ma.channelID, passthru.CLEAR_MSG_FILTERS, nil, nil); err != nil {
		return err
	}
	if err := ma.h.PassThruDisconnect(ma.channelID); err != nil {
		return err
	}
	if err := ma.h.PassThruClose(ma.deviceID); err != nil {
		return err
	}
	return ma.h.Close()
}

func (ma *J2534) Send(ctx context.Context, f gocan.Frame) error {
	var txflags uint32
	if f.Extended {
		txflags = passthru.CAN_29BIT_ID
	}
	msg := &passthru.PassThruMsg{
		ProtocolID:     ma.protocol,
		DataSize:       4 + uint32(f.Length),
		ExtraDataIndex: 4 + uint32(f.Length),
		TxFlags:        txflags,
	}
	if ma.protocol == passthru.SW_CAN_PS && !ma.tech2passThru {
		msg.TxFlags |= passthru.SW_CAN_HV_TX
	}
	binary.BigEndian.PutUint32(msg.Data[:], f.ID)
	copy(msg.Data[4:], f.Bytes())

	if ma.tech2passThru {
		ma.mu.Lock()
		defer ma.mu.Unlock()
	}
	numMsg := uint32(1)
	if err := ma.h.PassThruWriteMsgs(ma.channelID, msg, &numMsg, 25); err != nil {
		if errStr, err2 := ma.h.PassThruGetLastError(); err2 == nil {
			return fmt.Errorf("%w: %s", err, errStr)
		}
		return err
	}
	return nil
}

// SetFilter replaces the installed PASS filters at runtime.
func (ma *J2534) SetFilter(filters []uint32) error {
	if err := ma.h.PassThruClearMsgFilters(ma.channelID); err != nil {
		return err
	}
	ma.filters = filters
	if len(filters) > 0 {
		return ma.setupFilters()
	}
	ma.allowAll()
	return nil
}

func (ma *J2534) allowAll() {
	filterID := uint32(0)
	var txflags uint32
	if ma.cfg.UseExtendedID {
		txflags = passthru.CAN_29BIT_ID
	}
	maskMsg := &passthru.PassThruMsg{
		ProtocolID:     ma.protocol,
		DataSize:       4,
		ExtraDataIndex: 4,
		TxFlags:        txflags,
	}
	patternMsg := &passthru.PassThruMsg{
		ProtocolID:     ma.protocol,
		DataSize:       4,
		ExtraDataIndex: 4,
		TxFlags:        txflags,
	}
	if err := ma.h.PassThruStartMsgFilter(ma.channelID, passthru.PASS_FILTER, maskMsg, patternMsg, nil, &filterID); err != nil {
		ma.emit(gocan.EventTypeError, fmt.Sprintf("PassThruStartMsgFilter: %v", err))
	}
}

func (ma *J2534) setupFilters() error {
	if len(ma.filters) > 10 {
		return errors.New("too many filters")
	}
	var txflags uint32
	if ma.cfg.UseExtendedID {
		txflags = passthru.CAN_29BIT_ID
	}
	maskMsg := &passthru.PassThruMsg{
		ProtocolID:     ma.protocol,
		DataSize:       4,
		ExtraDataIndex: 4,
		Data:           [passthru.MSG_DATA_SIZE]byte{0x00, 0x00, 0xff, 0xff},
		TxFlags:        txflags,
	}
	for i, filter := range ma.filters {
		filterID := uint32(i)
		patternMsg := &passthru.PassThruMsg{
			ProtocolID:     ma.protocol,
			DataSize:       4,
			ExtraDataIndex: 4,
			TxFlags:        txflags,
		}
		binary.BigEndian.PutUint32(patternMsg.Data[:], filter)
		if err := ma.h.PassThruStartMsgFilter(ma.channelID, passthru.PASS_FILTER, maskMsg, patternMsg, nil, &filterID); err != nil {
			return err
		}
	}
	return nil
}

func (ma *J2534) readLoop(ctx context.Context) {
	for ctx.Err() == nil {
		msg, err := ma.readMsg()
		if err != nil {
			if ctx.Err() == nil {
				ma.emit(gocan.EventTypeError, err.Error())
			}
			continue
		}
		if msg == nil {
			continue
		}
		if msg.DataSize < 4 || msg.DataSize > 12 {
			ma.emit(gocan.EventTypeError, fmt.Sprintf("bad message size: %d", msg.DataSize))
			continue
		}
		f := gocan.Frame{
			ID:       binary.BigEndian.Uint32(msg.Data[0:4]),
			Length:   uint8(msg.DataSize - 4),
			Extended: msg.RxStatus&passthru.CAN_29BIT_ID != 0,
		}
		copy(f.Data[:], msg.Data[4:msg.DataSize])
		ma.bus.Deliver(f)
	}
}

func (ma *J2534) readMsg() (*passthru.PassThruMsg, error) {
	if ma.tech2passThru {
		ma.mu.Lock()
		defer ma.mu.Unlock()
	}
	msg := new(passthru.PassThruMsg)
	msg.ProtocolID = ma.protocol
	n, err := ma.h.PassThruReadMsg(ma.channelID, msg, 50)
	if err != nil {
		if errors.Is(err, passthru.ErrBufferEmpty) {
			return nil, nil
		}
		if errors.Is(err, passthru.ErrDeviceNotConnected) {
			return nil, fmt.Errorf("device not connected: %w", err)
		}
		return nil, fmt.Errorf("read error: %w", err)
	}
	if n == 0 {
		return nil, nil
	}
	return msg, nil
}

func (ma *J2534) emit(t gocan.EventType, details string) {
	ma.bus.Emit(gocan.Event{Type: t, Details: details})
}

//go:build kvaser

package adapter

/*
#cgo LDFLAGS: -lcanlib32
#include <canlib.h>
*/
import "C"

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"unsafe"

	"github.com/roffe/gocan"
)

func init() {
	//	log.Println("Kvaser adapter init")
	C.canInitializeLibrary()
	for channel := range GetChannels() {
		devDescr, err := GetDevDescr(channel)
		if err != nil {
			panic(err)
		}
		name := fmt.Sprintf("Canlib #%d %v", channel, devDescr)
		if err := Register(&AdapterInfo{
			Name:               name,
			Description:        "Canlib driver for Kvaser devices",
			RequiresSerialPort: false,
			Capabilities: AdapterCapabilities{
				HSCAN: true,
				KLine: false,
				SWCAN: false,
			},
			New: NewKvaser(channel, name),
		}); err != nil {
			panic(err)
		}
	}
}

const (
	defaultReadTimeoutMs  = 50
	defaultWriteTimeoutMs = defaultReadTimeoutMs
)

const (
	OpenExclusive         int = C.canOPEN_EXCLUSIVE           // Exclusive access
	OpenRequireExtended   int = C.canOPEN_REQUIRE_EXTENDED    // Fail if can't use extended mode
	OpenAcceptVirtual     int = C.canOPEN_ACCEPT_VIRTUAL      // Allow use of virtual CAN
	OpenOverrideExclusive int = C.canOPEN_OVERRIDE_EXCLUSIVE  // Open, even if in exclusive access
	OpenRequireInitAccess int = C.canOPEN_REQUIRE_INIT_ACCESS // Init access to bus
	OpenNoInitAccess      int = C.canOPEN_NO_INIT_ACCESS
	OpenAcceptLargeDlc    int = C.canOPEN_ACCEPT_LARGE_DLC
	OpenCanFd             int = C.canOPEN_CAN_FD
	OpenCanFdNonIso       int = C.canOPEN_CAN_FD_NONISO
	OpenInternalL         int = C.canOPEN_INTERNAL_L
)

const (
	StatusOk int = C.canOK
)

var (
	ErrNoMsg error = NewKvaserError(C.canERR_NOMSG)
	ErrArgs  error = errors.New("error in arguments")
)

type KvaserError struct {
	Code        int
	Description string
}

func (ke *KvaserError) Error() string {
	return fmt.Sprintf("%v (%v)", ke.Description, ke.Code)
}

func NewKvaserError(code int) error {
	if code >= StatusOk {
		return nil
	}
	msg := [64]C.char{}
	status := int(C.canGetErrorText(C.canStatus(code), &msg[0], C.uint(unsafe.Sizeof(msg))))
	if status < StatusOk {
		return fmt.Errorf("unable to get description for error code %v (%v)", code, status)
	}
	return &KvaserError{Code: code, Description: C.GoString(&msg[0])}
}

var _ gocan.Adapter = (*Kvaser)(nil)

type Kvaser struct {
	BaseAdapter
	channel      int
	handle       C.canHandle
	timeoutRead  int
	timeoutWrite int
}

func NewKvaser(channel int, name string) func(cfg *gocan.AdapterConfig) (gocan.Adapter, error) {
	return func(cfg *gocan.AdapterConfig) (gocan.Adapter, error) {
		return &Kvaser{
			channel:      channel,
			BaseAdapter:  NewBaseAdapter(name, cfg),
			timeoutRead:  defaultReadTimeoutMs,
			timeoutWrite: defaultWriteTimeoutMs,
		}, nil
	}
}

func (k *Kvaser) SetFilter(filters []uint32) error {
	return nil
}

func (k *Kvaser) Connect(ctx context.Context) error {
	if k.cfg.PrintVersion {
		version := C.canGetVersion()
		low := version & 0xFF
		high := version >> 8
		log.Printf("Canlib version: %v.%v", high, low)
	}

	flags := OpenExclusive

	handle := C.canOpenChannel(C.int(k.channel), C.int(flags))

	if err := NewKvaserError(int(handle)); err != nil {
		return fmt.Errorf("canOpenChannel error: %v", err)
	}
	k.handle = handle

	var err error
	switch k.cfg.CANRate {
	case 10:
		err = NewKvaserError(int(C.canSetBusParams(k.handle, C.canBITRATE_10K, 0, 0, 0, 0, 0)))
	case 50:
		err = NewKvaserError(int(C.canSetBusParams(k.handle, C.canBITRATE_50K, 0, 0, 0, 0, 0)))
	case 62:
		err = NewKvaserError(int(C.canSetBusParams(k.handle, C.canBITRATE_62K, 0, 0, 0, 0, 0)))
	case 83:
		err = NewKvaserError(int(C.canSetBusParams(k.handle, C.canBITRATE_83K, 0, 0, 0, 0, 0)))
	case 100:
		err = NewKvaserError(int(C.canSetBusParams(k.handle, C.canBITRATE_100K, 0, 0, 0, 0, 0)))
	case 125:
		err = NewKvaserError(int(C.canSetBusParams(k.handle, C.canBITRATE_125K, 0, 0, 0, 0, 0)))
	case 250:
		err = NewKvaserError(int(C.canSetBusParams(k.handle, C.canBITRATE_250K, 0, 0, 0, 0, 0)))
	case 500:
		err = NewKvaserError(int(C.canSetBusParams(k.handle, C.canBITRATE_500K, 0, 0, 0, 0, 0)))
	case 615.384:
		err = NewKvaserError(int(C.canSetBusParamsC200(k.handle, C.uchar(0x40), C.uchar(0x37))))
	case 1000:
		err = NewKvaserError(int(C.canSetBusParams(k.handle, C.canBITRATE_1M, 0, 0, 0, 0, 0)))
	default:
		return errors.New("unsupported CAN rate")
	}
	if err != nil {
		return fmt.Errorf("canSetBusParams error: %v", err)
	}
	if err := NewKvaserError(int(C.canSetBusOutputControl(k.handle, C.canDRIVER_NORMAL))); err != nil {
		return fmt.Errorf("canSetBusOutputControl error: %v", err)
	}

	go k.recvManager(ctx)
	go k.sendManager(ctx)

	return k.on()
}

func (k *Kvaser) recvManager(ctx context.Context) {
	defer log.Println("Kvaser.recvManager() done")
	for {
		select {
		case <-ctx.Done():
			return
		case <-k.close:
			return
		default:
			frame, err := k.recvFrame()
			if err != nil && err.Error() != ErrNoMsg.Error() {
				k.err <- fmt.Errorf("Kvaser.recvManager() error: %v", err)
				return
			}
			if frame == nil {
				continue
			}
			select {
			case k.recv <- frame:
			default:
				log.Println("Kvaser.recvManager() dropped frame")
			}
		}
	}
}

func (k *Kvaser) recvFrame() (gocan.CANFrame, error) {
	identifier := C.long(0)
	var data [8]byte
	dlc := C.uint(0)
	flags := C.uint(0)
	time := C.ulong(0)
	timeout := C.ulong(k.timeoutRead)

	status := C.canReadWait(k.handle, &identifier, unsafe.Pointer(&data), &dlc, &flags, &time, timeout)
	err := NewKvaserError(int(status))
	if err != nil {
		return nil, err
	}
	frame := gocan.NewFrame(uint32(identifier), data[:dlc], gocan.Incoming)
	return frame, nil
}

func (k *Kvaser) sendManager(ctx context.Context) {
	defer log.Println("Kvaser.sendManager() done")
	for {
		select {
		case <-ctx.Done():
			return
		case <-k.close:
			return
		case frame := <-k.send:
			if err := k.sendFrame(frame); err != nil {
				k.err <- fmt.Errorf("Kvaser.sendManager() error: %v", err)
				return
			}

		}
	}
}

func (k *Kvaser) sendFrame(frame gocan.CANFrame) error {
	id := C.long(frame.Identifier())
	data := frame.Data()
	return NewKvaserError(int(C.canWrite(k.handle, id, unsafe.Pointer(&data[0]), C.uint(len(data)), C.canMSG_STD)))
	//status = C.canWriteSync(k.handle, defaultWriteTimeoutMs)
	//if err := NewKvaserError(int(status)); err != nil {
	//	log.Println("canWriteSync error:", err)
	//	return
	//}
}

// Turn bus On
func (k *Kvaser) on() error {
	return NewKvaserError(int(C.canBusOn(k.handle)))
}

// Turn bus Off
func (k *Kvaser) off() error {
	return NewKvaserError(int(C.canBusOff(k.handle)))
}

func (k *Kvaser) Close() error {
	log.Println("Kvaser.Close()")
	k.BaseAdapter.Close()
	if err := k.off(); err != nil {
		log.Println("Kvaser.Close() off error:", err)
	}
	return NewKvaserError(int(C.canClose(k.handle)))
}

// Get number of channels
func GetChannels() int {
	nb := C.int(0)
	C.canGetNumberOfChannels(&nb)
	return int(nb)
}

func GetDevDescr(channel int) (string, error) {
	var name [256]byte
	if err := NewKvaserError(int(C.canGetChannelData(C.int(channel), C.canCHANNELDATA_DEVDESCR_ASCII, unsafe.Pointer(&name), 256))); err != nil {
		return "", err
	}
	return extractString(name), nil
}

func extractString(data [256]byte) string {
	if i := bytes.Index(data[:], []byte{0}); i != -1 {
		return string(data[:i])
	}
	return string(data[:])
}

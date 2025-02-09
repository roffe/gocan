//go:build kvaser

package adapter

/*
#cgo LDFLAGS: -lcanlib32
#include <canlib.h>
*/
import "C"

import (
	"context"
	"errors"
	"fmt"
	"log"
	"unsafe"

	"github.com/roffe/gocan"
)

func init() {
	if err := Register(&AdapterInfo{
		Name:               "Kvaser HS",
		Description:        "Canlib driver for Kvaser devices",
		RequiresSerialPort: false,
		Capabilities: AdapterCapabilities{
			HSCAN: true,
			KLine: false,
			SWCAN: false,
		},
		New: NewKvaser,
	}); err != nil {
		panic(err)
	}
}

const (
	defaultReadTimeoutMs  = 500
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

	handle       C.canHandle
	timeoutRead  int
	timeoutWrite int
}

func NewKvaser(cfg *gocan.AdapterConfig) (gocan.Adapter, error) {
	C.canInitializeLibrary()
	return &Kvaser{
		BaseAdapter:  NewBaseAdapter(cfg),
		timeoutRead:  defaultReadTimeoutMs,
		timeoutWrite: defaultWriteTimeoutMs,
	}, nil
}

func (k *Kvaser) SetFilter(filters []uint32) error {
	return nil
}

func (k *Kvaser) Name() string {
	return "Kvaser HS"
}

func (k *Kvaser) Init(ctx context.Context) error {
	if k.cfg.PrintVersion {
		// print version
		log.Println("Kvaser adapter")
	}
	channel := 0
	flags := OpenExclusive

	handle := C.canOpenChannel(C.int(channel), C.int(flags))
	err := NewKvaserError(int(handle))
	if err != nil {
		return err
	}
	k.handle = handle

	log.Println("CANRate:", k.cfg.CANRate)
	switch k.cfg.CANRate {
	case 500:
		err = NewKvaserError(int(C.canSetBusParams(k.handle, C.canBITRATE_500K, 0, 0, 0, 0, 0)))
	case 615.384:
		//err = NewKvaserError(int(C.canSetBusParams(k.handle, C.canBITRATE_500K, 0, 0, 0, 0, 0)))
		btr0 := C.uchar(0x40)
		btr1 := C.uchar(0x37)
		err = NewKvaserError(int(C.canSetBusParamsC200(k.handle, btr0, btr1)))
	default:
		return errors.New("unsupported CAN rate")
	}
	if err != nil {
		return err
	}
	err = NewKvaserError(int(C.canSetBusOutputControl(k.handle, C.canDRIVER_NORMAL)))
	if err != nil {
		return err
	}

	go k.recvManager(ctx)
	go k.sendManager(ctx)

	return k.On()
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
				log.Println("Kvaser.recvManager() error:", err)
				return
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
	id := C.long(0)
	var data [8]byte
	dlc := C.uint(0)
	flags := C.uint(0)
	time := C.ulong(0)
	timeout := C.ulong(k.timeoutRead)

	status := C.canReadWait(k.handle, &id, unsafe.Pointer(&data), &dlc, &flags, &time, timeout)
	err := NewKvaserError(int(status))
	if err != nil {
		return nil, err
	}
	frame := gocan.NewFrame(uint32(id), data[:dlc], gocan.Incoming)
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
			id := C.long(frame.Identifier())
			data := frame.Data()
			status := C.canWrite(k.handle, id, unsafe.Pointer(&data[0]), C.uint(len(data)), C.canMSG_STD)
			if err := NewKvaserError(int(status)); err != nil {
				log.Println("canWrite error:", err)
				return
			}
			//status = C.canWriteSync(k.handle, defaultWriteTimeoutMs)
			//if err := NewKvaserError(int(status)); err != nil {
			//	log.Println("canWriteSync error:", err)
			//	return
			//}
		}
	}

}

// Turn bus On
func (k *Kvaser) On() error {
	status := int(C.canBusOn(k.handle))
	return NewKvaserError(status)
}

// Turn bus Off
func (k *Kvaser) Off() error {
	status := int(C.canBusOff(k.handle))
	return NewKvaserError(status)
}

func (k *Kvaser) Close() error {
	log.Println("Kvaser.Close()")
	k.BaseAdapter.Close()
	k.Off()
	status := C.canClose(k.handle)
	return NewKvaserError(int(status))
}

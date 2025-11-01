package ftdi

import (
	"errors"
)

type BitMode byte

const (
	RESET         BitMode = 0x00
	ASYNC_BITBANG BitMode = 0x01
	MPSSE         BitMode = 0x02
	SYNC_BITBANG  BitMode = 0x04
	HOST_EMU      BitMode = 0x08
	FAST_OPTO     BitMode = 0x10
	CBUS_BITBANG  BitMode = 0x20
	SYNCHRONOUS   BitMode = 0x40
)

type FlowControl uint16

const (
	DISABLED FlowControl = 0x0000
	RTS_CTS  FlowControl = 0x0100
	DTR_DSR  FlowControl = 0x0200
	XON_XOFF FlowControl = 0x0400
)

type LineProperties struct {
	Bits     bitsPerWord
	StopBits stopBits
	Parity   parity
}
type bitsPerWord byte
type stopBits byte
type parity byte

const (
	BITS_8 bitsPerWord = 8
	BIST_7 bitsPerWord = 7
	STOP_1 stopBits    = 0
	STOP_2 stopBits    = 2
	NONE   parity      = 0
	ODD    parity      = 1
	EVEN   parity      = 2
	MARK   parity      = 3
	SPACE  parity      = 4
)

const (
	CHECK_RX_DELAY_MS = 200
)

// FT_STATUS (DWORD)
const (
	FT_OK                          = 0
	FT_INVALID_HANDLE              = 1
	FT_DEVICE_NOT_FOUND            = 2
	FT_DEVICE_NOT_OPENED           = 3
	FT_IO_ERROR                    = 4
	FT_INSUFFICIENT_RESOURCES      = 5
	FT_INVALID_PARAMETER           = 6
	FT_INVALID_BAUD_RATE           = 7
	FT_DEVICE_NOT_OPENED_FOR_ERASE = 8
	FT_DEVICE_NOT_OPENED_FOR_WRITE = 9
	FT_FAILED_TO_WRITE_DEVICE      = 10
	FT_EEPROM_READ_FAILED          = 11
	FT_EEPROM_WRITE_FAILED         = 12
	FT_EEPROM_ERASE_FAILED         = 13
	FT_EEPROM_NOT_PRESENT          = 14
	FT_EEPROM_NOT_PROGRAMMED       = 15
	FT_INVALID_ARGS                = 16
	FT_NOT_SUPPORTED               = 17
	FT_OTHER_ERROR                 = 18
)

var (
	ErrInvalidHandle           = errors.New("FTDI :: Invalid device handle")
	ErrDeviceNotFound          = errors.New("FTDI :: Device not found")
	ErrDeviceNotOpened         = errors.New("FTDI :: Device not opened")
	ErrIO                      = errors.New("FTDI :: IO failed")
	ErrInsufficientResources   = errors.New("FTDI :: Insufficient resources")
	ErrInvalidParameter        = errors.New("FTDI :: Invalid parameter")
	ErrInvalidBaudRate         = errors.New("FTDI :: Invalid baud rate")
	ErrDeviceNotOpenedForErase = errors.New("FTDI :: Device not opened for erase")
	ErrDeviceNotOpenedForWrite = errors.New("FTDI :: Device not opened for write")
	ErrFailedToWriteDevice     = errors.New("FTDI :: Failed to write device")
	ErrEReadFailed             = errors.New("FTDI :: EEPROM read failed")
	ErrEWriteFailed            = errors.New("FTDI :: EEPROM write failed")
	ErrEEraseFailed            = errors.New("FTDI :: EEPROM erase failed")
	ErrENotPresent             = errors.New("FTDI :: EEPROM not present")
	ErrENotProgrammed          = errors.New("FTDI :: EEPROM not programmed")
	ErrInvalidArgs             = errors.New("FTDI :: Invalid arguments")
	ErrNotSupported            = errors.New("FTDI :: Device not supported")
	ErrOther                   = errors.New("FTDI :: Unknown FTDI error")
)

var errorList = map[uintptr]error{
	FT_INVALID_HANDLE:              ErrInvalidHandle,
	FT_DEVICE_NOT_FOUND:            ErrDeviceNotFound,
	FT_DEVICE_NOT_OPENED:           ErrDeviceNotOpened,
	FT_IO_ERROR:                    ErrIO,
	FT_INSUFFICIENT_RESOURCES:      ErrInsufficientResources,
	FT_INVALID_PARAMETER:           ErrInvalidParameter,
	FT_INVALID_BAUD_RATE:           ErrInvalidBaudRate,
	FT_DEVICE_NOT_OPENED_FOR_ERASE: ErrDeviceNotOpenedForErase,
	FT_DEVICE_NOT_OPENED_FOR_WRITE: ErrDeviceNotOpenedForWrite,
	FT_FAILED_TO_WRITE_DEVICE:      ErrFailedToWriteDevice,
	FT_EEPROM_READ_FAILED:          ErrEReadFailed,
	FT_EEPROM_WRITE_FAILED:         ErrEWriteFailed,
	FT_EEPROM_ERASE_FAILED:         ErrEEraseFailed,
	FT_EEPROM_NOT_PRESENT:          ErrENotPresent,
	FT_EEPROM_NOT_PROGRAMMED:       ErrENotProgrammed,
	FT_INVALID_ARGS:                ErrInvalidArgs,
	FT_NOT_SUPPORTED:               ErrNotSupported,
	FT_OTHER_ERROR:                 ErrOther,
}

/* API
// Search the system for all connected FTDI devices.
// Returns a slice of `DeviceInfo` objects for each.
func GetDeviceList() []DeviceInfo {}

// Open the device described by DeviceInfo
func Open(di *DeviceInfo) *Device, error {}

// Close the device
// Return nil on success, error otherwise.
func (d *Device) Close() error {}

// Read from the device. Implements io.Reader.
func (d *Device) Read([]byte) int, error {}
// Write to the device. Implements io.Writer.
func (d *Device) Write([]byte) int, error {}

// Set the baudrate/bitrate in bits-per-second.
// Return nil on success, error otherwise.
func (d *Device) SetBaudrate(baud uint) error {}

// Set the device's bit mode.
func (d *Device) SetBitMode(mode BitMode) error {}
func (d *Device) SetFlowControl() error{}
func (d *Device) SetLatency() {}
func (d *Device) SetLineProperty() {}
func (d *Device) SetTimeout() {}
func (d *Device) SetTransferSize() {}
func (d *Device) SetChars() {}

// Reset the device. Returns nil on success,
// error otherwise.
func (d *Device) Reset() error {}
func (d *Device) Purge() error {}
*/

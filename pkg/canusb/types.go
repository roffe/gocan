package canusb

import "fmt"

type CANHANDLE struct {
	h int
}

const (
	// Status bits
	CANSTATUS_RECEIVE_FIFO_FULL  = 0x01
	CANSTATUS_TRANSMIT_FIFO_FULL = 0x02
	CANSTATUS_ERROR_WARNING      = 0x04
	CANSTATUS_DATA_OVERRUN       = 0x08
	CANSTATUS_ERROR_PASSIVE      = 0x20
	CANSTATUS_ARBITRATION_LOST   = 0x40
	CANSTATUS_BUS_ERROR          = 0x80

	// Filter mask settings
	ACCEPTANCE_CODE_ALL = 0x00000000
	ACCEPTANCE_MASK_ALL = 0xFFFFFFFF
)

type CANUsbStatistics struct {
	ReceiveFrames  uint32 // # of receive frames
	TransmitFrames uint32 // # of transmitted frames
	ReceiveData    uint32 // # of received data bytes
	TransmitData   uint32 // # of transmitted data bytes
	Overruns       uint32 // # of overruns
	BusWarnings    uint32 // # of bys warnings
	BusOff         uint32 // # of bus off's
}

func (c *CANUsbStatistics) String() string {
	return fmt.Sprintf("RF: %d, TF: %d, RB: %d, TB: %d, Overruns: %d, BusWarnings: %d, BusOff: %d",
		c.ReceiveFrames, c.TransmitFrames, c.ReceiveData, c.TransmitData, c.Overruns, c.BusWarnings, c.BusOff)
}

type FlushFlag uint8

// Flush flags
const (
	FLUSH_WAIT          FlushFlag = 0x00
	FLUSH_DONTWAIT      FlushFlag = 0x01
	FLUSH_EMPTY_INQUEUE FlushFlag = 0x02
)

type MessageFlag uint32

// Message flags
const (
	CANMSG_EXTENDED MessageFlag = 0x80 // Extended CAN id
	CANMSG_RTR      MessageFlag = 0x40 // Remote frame
)

//export CANMsg
type CANMsg struct {
	Id        uint32  // Message id
	Timestamp uint32  // timestamp in milliseconds
	Flags     uint8   // [extended_id|1][RTR:1][reserver:6]
	Len       uint8   // Frame size (0.8)
	Data      [8]byte // Databytes 0..7
}

func (msg *CANMsg) String() string {
	return fmt.Sprintf("ID: 0x%X, Timestamp: %d, Flags: %X, Len: %d, Data: %02X", msg.Id, msg.Timestamp, msg.Flags, msg.Len, msg.Data)
}

type OpenFlag uint32

const (
	FLAG_TIMESTAMP     OpenFlag = 0x0001 // Timestamp messages
	FLAG_QUEUE_REPLACE OpenFlag = 0x0002 // If input queue is full remove oldest message and insert new message.
	FLAG_BLOCK         OpenFlag = 0x0004 // Block receive/transmit
	FLAG_SLOW          OpenFlag = 0x0008 // // Check ACK/NACK's
	FLAG_NO_LOCAL_SEND OpenFlag = 0x0010 // Don't send transmited frames on other local channels for the same interface
)

type CallbackFunc func(msg *CANMsg) uintptr

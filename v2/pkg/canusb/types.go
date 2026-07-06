package canusb

import "fmt"

const (
	// Filter mask settings
	ACCEPTANCE_CODE_ALL uint32 = 0x00000000
	ACCEPTANCE_MASK_ALL uint32 = 0xFFFFFFFF
)

// CANHANDLE is a handle to a CANUSB device
type CANHANDLE struct {
	h int32
}

// Callback function used with SetReceiveCallback
type CallbackFunc func(msg *CANMsg) uintptr

const MAX_DLC = 8

// CAN Frame
type CANMsg struct {
	ID        uint32        // Message id
	Timestamp uint32        // timestamp in milliseconds
	Flags     MessageFlag   // Message flags
	Len       uint8         // Frame size (0.8)
	Data      [MAX_DLC]byte // Databytes 0..7
}

// Returns a string representation of the CANMsg
func (msg *CANMsg) String() string {
	return fmt.Sprintf("ID: 0x%X, Timestamp: %d, Flags: %2X, Len: %d, Data: % 2X", msg.ID, msg.Timestamp, msg.Flags, msg.Len, msg.Data[:msg.Len])
}

// Returns the data bytes of the CANMsg
func (msg *CANMsg) Bytes() []byte {
	return msg.Data[:min(msg.Len, 8)]
}

type CANMsgEx struct {
	ID        uint32      // Message id
	Timestamp uint32      // timestamp in milliseconds
	Flags     MessageFlag // Message flags
	Len       uint8       // Frame size (0.8)
}

// Returns a string representation of the CANMsgEx
func (msg *CANMsgEx) String() string {
	return fmt.Sprintf("ID: 0x%X, Timestamp: %d, Flags: %2X, Len: %d", msg.ID, msg.Timestamp, msg.Flags, msg.Len)
}

type MessageFlag uint8

// Message flags
const (
	CANMSG_STANDARD MessageFlag = 0x00 // Standard 11-bit CAN id
	CANMSG_EXTENDED MessageFlag = 0x80 // Extended 29-bit CAN id
	CANMSG_RTR      MessageFlag = 0x40 // Remote frame
)

const (
	// Status bits
	CANSTATUS_RECEIVE_FIFO_FULL  = 0x01
	CANSTATUS_TRANSMIT_FIFO_FULL = 0x02
	CANSTATUS_ERROR_WARNING      = 0x04
	CANSTATUS_DATA_OVERRUN       = 0x08
	CANSTATUS_ERROR_PASSIVE      = 0x20
	CANSTATUS_ARBITRATION_LOST   = 0x40
	CANSTATUS_BUS_ERROR          = 0x80
)

type CANUSBStatistics struct {
	ReceiveFrames  uint32 // # of receive frames
	TransmitFrames uint32 // # of transmitted frames
	ReceiveData    uint32 // # of received data bytes
	TransmitData   uint32 // # of transmitted data bytes
	Overruns       uint32 // # of overruns
	BusWarnings    uint32 // # of bus warnings
	BusOff         uint32 // # of bus off's
}

// Returns a string representation of the CANUsbStatistics
// 	- RF - # of Received frames
// 	- TF - # of Transmitted frames
// 	- RB - # of Received data bytes
// 	- TB - # of Transmitted data bytes
// 	- Overruns - # of overruns
// 	- BusWarnings - # of bus warnings
// 	- BusOff - # of bus off's
func (c *CANUSBStatistics) String() string {
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

type OpenFlag uint32

const (
	FLAG_TIMESTAMP     OpenFlag = 0x0001 // Timestamp messages
	FLAG_QUEUE_REPLACE OpenFlag = 0x0002 // If input queue is full remove oldest message and insert new message.
	FLAG_BLOCK         OpenFlag = 0x0004 // Block receive/transmit
	FLAG_SLOW          OpenFlag = 0x0008 // Check ACK/NACK's
	FLAG_NO_LOCAL_SEND OpenFlag = 0x0010 // Don't send transmited frames on other local channels for the same interface
)

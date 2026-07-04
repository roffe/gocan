package gocan

import (
	"fmt"
	"strings"
)

// CANFrameType tells the adapter how to treat an outgoing frame: fire and
// forget (Outgoing) or, on buffered adapters like the ELM/STN family, wait
// for Responses response frames (ResponseRequired).
type CANFrameType struct {
	Type      ResponseType
	Responses int
}

type ResponseType int

const (
	ResponseTypeIncoming         ResponseType = 0
	ResponseTypeOutgoing         ResponseType = 1
	ResponseTypeResponseRequired ResponseType = 2 // Used for ELM and STN adapters to signal we want the adapter to wait for a response
)

// Treat these as constants; mutating them affects every user in the process.
var (
	Incoming         = CANFrameType{Type: ResponseTypeIncoming, Responses: 0}
	Outgoing         = CANFrameType{Type: ResponseTypeOutgoing, Responses: 0}
	ResponseRequired = CANFrameType{Type: ResponseTypeResponseRequired, Responses: 1} // Used for ELM and STN adapters to signal we want the adapter to wait for a response
)

// ResponseRequiredWithResponses is ResponseRequired for commands that yield
// more than one response frame.
func ResponseRequiredWithResponses(responses int) CANFrameType {
	return CANFrameType{Type: ResponseTypeResponseRequired, Responses: responses}
}

// CANFrame is a single CAN bus frame.
//
// Frames are single-use: the send methods attach per-send state (SendAndWait
// stamps Timeout, SendSync installs a completion signal), so build a fresh
// frame for every send instead of reusing one, especially across goroutines.
// Frames received from the bus are shared by every matching subscriber and
// must be treated as read-only, including the Data slice.
type CANFrame struct {
	Identifier uint32
	Extended   bool
	RTR        bool
	Data       []byte
	FrameType  CANFrameType
	// Timeout in milliseconds is a hint for buffered adapters waiting on a
	// response. It is stamped by Client.SendAndWait.
	Timeout uint32
	// sent is non-nil only for frames sent via Client.SendSync. The adapter
	// signals it once the frame has been written to the hardware.
	sent chan struct{}
}

// markSent notifies a SendSync waiter that the adapter has finished writing this
// frame. Safe to call on any frame (no-op unless the frame was sent via SendSync)
// and safe to call more than once.
func (f *CANFrame) markSent() {
	if f.sent != nil {
		select {
		case f.sent <- struct{}{}:
		default:
		}
	}
}

// NewExtendedFrame creates a new 29-bit CANFrame and copies the data slice.
func NewExtendedFrame(identifier uint32, data []byte, frameType CANFrameType) *CANFrame {
	frame := NewFrame(identifier, data, frameType)
	frame.Extended = true
	return frame
}

// NewFrame creates a new CANFrame and copies the data slice.
func NewFrame(identifier uint32, data []byte, frameType CANFrameType) *CANFrame {
	d := make([]byte, len(data))
	copy(d, data)
	return &CANFrame{
		Identifier: identifier,
		Data:       d,
		FrameType:  frameType,
	}
}

// DLC returns the length of the data.
func (f *CANFrame) DLC() int {
	return len(f.Data)
}

func (f *CANFrame) String() string {
	var out strings.Builder
	switch f.FrameType.Type {
	case 0:
		out.WriteString("<i> || ")
	case 1:
		out.WriteString("<o> || ")
	case 2:
		out.WriteString("<r> || ")

	}
	fmt.Fprintf(&out, "0x%03X || ", f.Identifier)
	fmt.Fprintf(&out, "%d || ", len(f.Data))
	var hexView strings.Builder
	for i, b := range f.Data {
		hexView.WriteString(fmt.Sprintf("%02X", b))
		if i != len(f.Data)-1 {
			hexView.WriteString(" ")
		}
	}
	fmt.Fprintf(&out, "%-23s", hexView.String())
	out.WriteString(" || ")
	var binView strings.Builder
	for i, b := range f.Data {
		binView.WriteString(fmt.Sprintf("%08b", b))
		if i != len(f.Data)-1 {
			binView.WriteString(" ")
		}
	}
	fmt.Fprintf(&out, "%-72s", binView.String())
	out.WriteString(" || ")
	out.WriteString(onlyPrintable(f.Data))
	return out.String()
}

func onlyPrintable(data []byte) string {
	var out strings.Builder
	for _, b := range data {
		if b < 32 || b > 127 {
			out.WriteString("·")
		} else {
			out.WriteByte(b)
		}
	}
	return out.String()
}

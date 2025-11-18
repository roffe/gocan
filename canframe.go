package gocan

import (
	"encoding/binary"
	"fmt"
	"strconv"
	"strings"

	"github.com/fatih/color"
)

type CANFrameType struct {
	Type      int
	Responses int
}

var (
	Incoming         = CANFrameType{Type: 0, Responses: 0}
	Outgoing         = CANFrameType{Type: 1, Responses: 0}
	ResponseRequired = CANFrameType{Type: 2, Responses: 1} // Used for ELM and STN adapters to signal we want the adapter to wait for a response
)

type CANFrame struct {
	Identifier uint32
	Extended   bool
	RTR        bool
	Data       []byte
	FrameType  CANFrameType
	Timeout    uint32
}

// NewExtendedFrame creates a new CANFrame and copies the data slice
func NewExtendedFrame(identifier uint32, data []byte, frameType CANFrameType) *CANFrame {
	frame := NewFrame(identifier, data, frameType)
	frame.Extended = true
	return frame
}

// NewFrame creates a new CANFrame and copies the data slice
func NewFrame(identifier uint32, data []byte, frameType CANFrameType) *CANFrame {
	d := make([]byte, len(data))
	copy(d, data)
	return &CANFrame{
		Identifier: identifier,
		Data:       d,
		FrameType:  frameType,
	}
}

// Returns the length of the data (DLC)
func (f *CANFrame) DLC() int {
	return len(f.Data)
}

var (
	yellow = color.New(color.FgHiBlue).SprintfFunc()
	red    = color.New(color.FgRed).SprintfFunc()
	green  = color.New(color.FgGreen).SprintfFunc()
	//printable = regexp.MustCompile("[^A-Za-z0-9.,!?]+")
)

// returns the frame as a byte slice, 4 bytes for the identifier, 1 byte for the length and the data
// if holding more than 8 bytes of data, it will be truncated
func (f *CANFrame) Bytes() []byte {
	data := make([]byte, 4, 13)
	dataLen := min(len(f.Data), 8)
	binary.LittleEndian.PutUint32(data, f.Identifier)
	binary.Append(data, binary.LittleEndian, uint8(dataLen))
	data = append(data, f.Data[:dataLen]...)
	return data
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
	out.WriteString(fmt.Sprintf("0x%03X", f.Identifier) + " || ")
	out.WriteString(strconv.Itoa(len(f.Data)) + " || ")
	var hexView strings.Builder
	for i, b := range f.Data {
		hexView.WriteString(fmt.Sprintf("%02X", b))
		if i != len(f.Data)-1 {
			hexView.WriteString(" ")
		}
	}
	out.WriteString(fmt.Sprintf("%-23s", hexView.String()))
	out.WriteString(" || ")
	var binView strings.Builder
	for i, b := range f.Data {
		binView.WriteString(fmt.Sprintf("%08b", b))
		if i != len(f.Data)-1 {
			binView.WriteString(" ")
		}
	}
	out.WriteString(fmt.Sprintf("%-72s", binView.String()))
	out.WriteString(" || ")
	out.WriteString(onlyPrintable(f.Data))
	return out.String()
}

func (f *CANFrame) ColorString() string {
	var out strings.Builder

	switch f.FrameType.Type {
	case 0:
		out.WriteString("<i> || ")
	case 1:
		out.WriteString("<o> || ")
	case 2:
		out.WriteString("<r> || ")

	}

	out.WriteString(green("0x%03X", f.Identifier) + " || ")

	out.WriteString(strconv.Itoa(len(f.Data)) + " || ")

	var hexView strings.Builder

	for i, b := range f.Data {
		hexView.WriteString(fmt.Sprintf("%02X", b))
		if i != len(f.Data)-1 {
			hexView.WriteString(" ")
		}
	}

	out.WriteString(fmt.Sprintf("%-23s", hexView.String()))

	out.WriteString(" || ")

	var binView strings.Builder
	for i, b := range f.Data {
		binView.WriteString(fmt.Sprintf("%08b", b))
		if i != len(f.Data)-1 {
			binView.WriteString(" ")
		}
	}

	out.WriteString(red(fmt.Sprintf("%-72s", binView.String())))

	out.WriteString(" || ")
	out.WriteString(yellow(onlyPrintable(f.Data)))
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

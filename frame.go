package gocan

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/fatih/color"
)

type CANFrameType struct {
	Type      int
	Responses int
}

func (c *CANFrameType) SetResponseCount(no int) {
	c.Responses = no
}

func (c *CANFrameType) GetResponseCount() int {
	return c.Responses
}

var (
	Incoming = CANFrameType{Type: 0, Responses: 0}
	Outgoing = CANFrameType{Type: 1, Responses: 0}
	// Used for ELM and STN adapters to signal we want the adapter to wait for a response
	ResponseRequired = CANFrameType{Type: 2, Responses: 1}
)

type CANFrame struct {
	Identifier uint32
	Extended   bool
	Data       []byte
	FrameType  CANFrameType
	Timeout    time.Duration
}

func NewExtendedFrame(identifier uint32, data []byte, frameType CANFrameType) *CANFrame {
	return &CANFrame{
		Identifier: identifier,
		Data:       data,
		FrameType:  frameType,
		Extended:   true,
	}
}

func NewFrame(identifier uint32, data []byte, frameType CANFrameType) *CANFrame {
	return &CANFrame{
		Identifier: identifier,
		Data:       data,
		FrameType:  frameType,
	}
}

func (f *CANFrame) Length() int {
	return len(f.Data)
}

var (
	yellow = color.New(color.FgHiBlue).SprintfFunc()
	red    = color.New(color.FgRed).SprintfFunc()
	green  = color.New(color.FgGreen).SprintfFunc()
	//printable = regexp.MustCompile("[^A-Za-z0-9.,!?]+")
)

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

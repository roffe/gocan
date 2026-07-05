package gocan

import (
	"fmt"
	"strings"
)

// Frame is a single classic CAN frame. It is a plain value: copy it freely,
// reuse it across sends and share it between goroutines without locking.
type Frame struct {
	ID       uint32
	Extended bool // 29-bit identifier
	Remote   bool // remote transmission request
	Length   uint8
	Data     [8]byte
}

// NewFrame builds an 11-bit frame carrying data. Data beyond 8 bytes is
// truncated; classic CAN cannot carry more.
func NewFrame(id uint32, data []byte) Frame {
	f := Frame{ID: id}
	f.Length = uint8(copy(f.Data[:], data))
	return f
}

// NewExtendedFrame builds a 29-bit frame carrying data. Data beyond 8 bytes
// is truncated.
func NewExtendedFrame(id uint32, data []byte) Frame {
	f := NewFrame(id, data)
	f.Extended = true
	return f
}

// Bytes returns the payload as a slice over the frame's Data array.
func (f *Frame) Bytes() []byte {
	return f.Data[:f.Length]
}

func (f Frame) String() string {
	var out strings.Builder
	fmt.Fprintf(&out, "0x%03X || %d || ", f.ID, f.Length)
	var hexView strings.Builder
	for i, b := range f.Data[:f.Length] {
		if i > 0 {
			hexView.WriteString(" ")
		}
		fmt.Fprintf(&hexView, "%02X", b)
	}
	fmt.Fprintf(&out, "%-23s || ", hexView.String())
	for _, b := range f.Data[:f.Length] {
		if b < 32 || b > 127 {
			out.WriteString("·")
		} else {
			out.WriteByte(b)
		}
	}
	return out.String()
}

package canusb

import (
	"fmt"
	"strings"
)

type Frame struct {
	Identifier uint32
	Len        uint8
	Data       []byte
}

func (f *Frame) Byte() []byte {
	//fmt.Println(f.String())
	return []byte(fmt.Sprintf("t%x%d%x\r", f.Identifier, f.Len, f.Data))
}

func (f *Frame) String() string {
	var out strings.Builder
	out.WriteString(fmt.Sprintf("0x%X", f.Identifier) + " [")
	for i, b := range f.Data {
		out.WriteString(fmt.Sprintf("%02X", b))
		if i != len(f.Data)-1 {
			out.WriteString(" ")
		}
	}
	out.WriteString("] || ")
	out.WriteString(fmt.Sprintf("%q", f.Data))
	return out.String()
}

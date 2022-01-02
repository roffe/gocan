package canusb

import "fmt"

type Frame struct {
	Identifier uint16
	Len        uint8
	Data       []byte
	processed  chan struct{}
}

func (f *Frame) String() string {
	return fmt.Sprintf("0x%x %d %X", f.Identifier, f.Len, f.Data)
}

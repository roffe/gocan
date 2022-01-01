package canusb

import "fmt"

type Frame struct {
	Identifier string
	Len        uint64
	Data       []byte
}

func (f *Frame) String() string {
	return fmt.Sprintf("%s %d %X", string(f.Identifier), f.Len, f.Data)
}

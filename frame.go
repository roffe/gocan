package canusb

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync/atomic"
)

type Frame struct {
	Identifier uint16
	Len        uint8
	Data       []byte
	processed  chan struct{}
}

func (f *Frame) Send(c *Canusb) error {
	defer close(f.processed)
	out := fmt.Sprintf("t%x%d%x", f.Identifier, f.Len, f.Data)
	n, err := c.port.Write(B(out + "\r"))
	if err != nil {
		return fmt.Errorf("failed to send frame %s: %v", f.String(), err)
	}
	atomic.AddUint64(&c.sentBytes, uint64(n))
	return nil
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
	out.WriteString("]")
	return out.String()
}

func parseFrame(buff *bytes.Buffer) *Frame {
	p := strings.ReplaceAll(buff.String(), "\r", "")
	b, err := hex.DecodeString(fmt.Sprintf("%04s", p[1:4]))
	if err != nil {
		log.Fatal(err)
	}
	addr := binary.BigEndian.Uint16(b)
	len, err := strconv.ParseUint(string(p[4:5]), 0, 8)
	if err != nil {
		log.Fatal(err)
	}
	data, err := hex.DecodeString(p[5:])
	if err != nil {
		log.Fatal(err)
	}
	return &Frame{
		Identifier: addr,
		Len:        uint8(len),
		Data:       data,
	}

}

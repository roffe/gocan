package dvi

import (
	"fmt"
)

const (
	FRAME_TYPE_11BIT           = 0x00
	FRAME_TYPE_29BIT           = 0x01
	FILTER_TYPE_PASS           = 0x00
	FILTER_TYPE_FLOW           = 0x01
	FILTER_TYPE_BLOCK          = 0x02
	FILTER_STATUS_OFF          = 0x00
	FILTER_STATUS_ON           = 0x01
	CMD_SEND_TO_NETWORK_NORMAL = 0x10
)

func Parse(data []byte) (*Command, error) {
	if len(data) < 3 {
		return nil, fmt.Errorf("data too short")
	}
	cmd := &Command{
		command:  data[0],
		length:   data[1],
		data:     data[2 : 2+data[1]],
		checksum: data[2+data[1]],
	}

	//	log.Printf("data: %X", cmd.data)

	if calulateChecksum(cmd) != cmd.checksum {
		return nil, fmt.Errorf("checksum error")
	}

	return cmd, nil
}

func (c *Command) Length() int {
	return int(c.length)
}

func New(command byte, data []byte) *Command {
	cmd := &Command{
		command: command,
		length:  byte(len(data)),
		data:    data,
	}
	cmd.checksum = calulateChecksum(cmd)
	return cmd
}

type Command struct {
	command  byte   // Command byte
	length   byte   // Length of bytes to come (Only includes data bytes, excludes checksum)
	data     []byte // Data byte(s)
	checksum byte   // Checksum (Sum of bytes and inversed)
}

func (c *Command) Command() byte {
	return c.command
}

func (c *Command) Data() []byte {
	return c.data
}

func (c *Command) Checksum() byte {
	return c.checksum
}

func (c *Command) String() string {
	return fmt.Sprintf("%02X:, Length: %02d, Data: %X, Checksum: %02X", c.command, c.length, c.data, c.checksum)
}

func calulateChecksum(d *Command) byte {
	var sum byte
	sum = d.command + d.length
	for _, b := range d.data {
		sum += b
	}
	return ^sum
}

func (d *Command) Bytes() []byte {
	return append(append([]byte{d.command, d.length}, d.data...), d.checksum)
}

package canusb

import (
	"log"
)

func (c *Canusb) SendFrame(identifier uint16, data []byte) {
	c.Send(&Frame{
		Identifier: identifier,
		Len:        uint8(len(data)),
		Data:       data,
	})
}

func (c *Canusb) SendQuery() {
	err := c.Send(&Frame{
		Identifier: 0x220,
		Len:        8,
		Data:       B{0x3f, 0x81, 0x01, 0x33, 0x02, 0x40, 0x00, 0x00},
	})
	if err != nil {
		log.Fatal(err)
	}
}

func (c *Canusb) SecMsg() {
	c.Send(&Frame{
		Identifier: 0x240,
		Len:        8,
		Data:       SecurityMsg,
	})
}

func (c *Canusb) SecMsg2() {
	c.Send(&Frame{
		Identifier: 0x240,
		Len:        8,
		Data:       SecurityMsgReply,
	})
}

func (c *Canusb) SecACK() {
	c.Send(&Frame{
		Identifier: 0x240,
		Len:        8,
		Data:       Ack,
	})
}

var Ack = B{0x40, 0xA1, 0x3F, 0x00, 0x00, 0x00, 0x00, 0x00}
var SecurityMsgReply = B{0x40, 0xA1, 0x04, 0x27, 0x06, 0x00, 0x00, 0x00}
var SecurityMsg = B{0x40, 0xA1, 0x02, 0x27, 0x05, 0x00, 0x00, 0x00}

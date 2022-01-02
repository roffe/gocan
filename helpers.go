package canusb

import (
	"bytes"
	"log"
	"time"
)

func (c *Canusb) Tjong(method int) bool {
	securityMsg := []byte{0x40, 0xA1, 0x02, 0x27, 0x05, 0x00, 0x00, 0x00}
	securityMsgReply := []byte{0x40, 0xA1, 0x04, 0x27, 0x06, 0x00, 0x00, 0x00}
	ack := []byte{0x40, 0xA1, 0x3F, 0x00, 0x00, 0x00, 0x00, 0x00}

	c.SendFrame(0x240, securityMsg)

	f, err := c.WaitForFrame(0x258, 100*time.Millisecond)
	if err != nil {
		log.Println(err)
		return false

	}
	ack[3] = f.Data[0] & 0xBF
	c.SendFrame(0x266, ack)

	seed := int(f.Data[5])<<8 | int(f.Data[6])
	key := calcAuthKey(seed, method)

	securityMsgReply[5] = byte(int(key) >> 8 & int(0xFF))
	securityMsgReply[6] = byte(key) & 0xFF

	c.SendFrame(0x240, securityMsgReply)
	f2, err := c.WaitForFrame(0x258, 100*time.Millisecond)
	if err != nil {
		log.Println(err)
		return false

	}
	ack[3] = f2.Data[0] & 0xBF
	c.SendFrame(0x266, ack)
	if f2.Data[3] == 0x67 && f2.Data[5] == 0x34 {
		log.Println("Motherfucking säkerthetsaccess GRANTED")
		return true
	} else {
		log.Println("Sug röv, ingen säkerhets access...")
		return false
	}
}

func calcAuthKey(seed int, method int) int {
	key := seed << 2
	key &= 0xFFFF
	switch method {
	case 0:
		key ^= 0x8142
		key -= 0x2356
	case 1:
		key ^= 0x4081
		key -= 0x1F6F
	}
	key &= 0xFFFF
	return key
}

func (c *Canusb) GetHeader(id byte) string {
	var answer []byte
	var length int
	more := true

	go func() {
		time.Sleep(10 * time.Millisecond)
		c.SendFrame(0x240, B{0x40, 0xA1, 0x02, 0x1A, id, 0x00, 0x00, 0x00})
	}()

	for more {
		f, err := c.WaitForFrame(0x258, 100*time.Millisecond)
		if err != nil {
			log.Println(err)
			continue

		}
		if f.Data[0]&0x40 == 0x40 {
			if int(f.Data[2]) > 2 {
				length = int(f.Data[2]) - 2
			}
			for i := 5; i < 8; i++ {
				if length > 0 {
					answer = append(answer, f.Data[i])
				}
				length--
			}
		} else {
			for i := 0; i < 6; i++ {
				if length == 0 {
					break
				}
				answer = append(answer, f.Data[2+i])
				length--
			}

		}

		c.SendFrame(0x266, B{0x40, 0xA1, 0x3F, f.Data[0] & 0xBF, 0x00, 0x00, 0x00, 0x00})
		if bytes.Equal(f.Data[:1], B{0x80}) || bytes.Equal(f.Data[:1], B{0xC0}) {
			more = false
		}
	}

	//log.Printf("0x%02x [%d] %q\n", id, len(answer), string(answer))
	return string(answer)
}

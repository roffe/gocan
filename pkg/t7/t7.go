package t7

import (
	"bytes"
	"fmt"
	"log"
	"time"

	"github.com/avast/retry-go"
	"github.com/roffe/canusb"
)

const (
	IBusRate = 47.619
	PBusRate = 500
)

func DecodeSaabFrame(f *canusb.Frame) {
	//https://pikkupossu.1g.fi/tomi/projects/p-bus/p-bus.html
	var prefix string
	var signfBit bool
	switch f.Identifier {
	case 0x238: // Trionic data initialization reply
		prefix = "TDI"
	case 0x240: //  Trionic data query
		prefix = "TDIR"
	case 0x258: // Trionic data query reply
		prefix = "TDQR"
	case 0x266: // Trionic reply acknowledgement
		prefix = "TRA"
	case 0x370: // Mileage
		prefix = "MLG"
	case 0x3A0: // Vehicle speed (MIU?)
		prefix = "MIU"
	case 0x1A0: // Engine information
		signfBit = true
		prefix = "ENG"
	default:
		prefix = "UNK"
	}
	if signfBit {
		log.Printf("%s> 0x%x  %d  %08b  %X\n", prefix, f.Identifier, f.Len, f.Data[:1], f.Data[1:])
		return
	}
	log.Printf("in> %s> 0x%x  %d %X\n", prefix, f.Identifier, f.Len, f.Data)
}

func Dumperino(c *canusb.Canusb) {
	err := retry.Do(
		func() error {
			go func() {
				time.Sleep(100 * time.Millisecond)
				c.SendFrame(0x220, canusb.B{0x3F, 0x81, 0x00, 0x11, 0x02, 0x40, 0x00, 0x00}) //init:msg
			}()
			_, err := c.Poll(0x238, 1200*time.Millisecond)
			if err != nil {
				return fmt.Errorf("%v", err)
			}
			return nil

		},
		retry.Attempts(5),
		retry.Delay(100*time.Millisecond),
		retry.OnRetry(func(n uint, err error) {
			log.Printf("#%d: %s\n", n, err.Error())
		}),
	)
	if err != nil {
		log.Println(err)
		return
	}

	var authed bool
	for i := 0; i < 2; i++ {
		if LetMeIn(c, i) {
			log.Println("Trusted access obtained 🥳🎉")
			authed = true
			break
		}
	}
	if !authed {
		log.Println("/!\\ failed to obtain security access 😞👎🏻")
	}

	log.Println("VIN:", GetHeader(c, 0x90))
	log.Println("Box HW part number:", GetHeader(c, 0x91))
	log.Println("Immo Code:", GetHeader(c, 0x92))
	log.Println("Software Saab part number:", GetHeader(c, 0x94))
	log.Println("ECU Software version:", GetHeader(c, 0x95))
	log.Println("Engine type:", GetHeader(c, 0x97))
	log.Println("Tester info:", GetHeader(c, 0x98))
	log.Println("Software date:", GetHeader(c, 0x99))

}

func GetHeader(c *canusb.Canusb, id byte) string {
	var answer []byte
	var length int
	c.SendFrame(0x240, canusb.B{0x40, 0xA1, 0x02, 0x1A, id, 0x00, 0x00, 0x00})
	for {
		f, err := c.Poll(0x258, 150*time.Millisecond)
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

		c.SendFrame(0x266, canusb.B{0x40, 0xA1, 0x3F, f.Data[0] & 0xBF, 0x00, 0x00, 0x00, 0x00})
		if bytes.Equal(f.Data[:1], canusb.B{0x80}) || bytes.Equal(f.Data[:1], canusb.B{0xC0}) {
			break
		}
	}
	return string(answer)
}

func LetMeIn(c *canusb.Canusb, method int) bool {
	msg := []byte{0x40, 0xA1, 0x02, 0x27, 0x05, 0x00, 0x00, 0x00}
	msgReply := []byte{0x40, 0xA1, 0x04, 0x27, 0x06, 0x00, 0x00, 0x00}
	ack := []byte{0x40, 0xA1, 0x3F, 0x00, 0x00, 0x00, 0x00, 0x00}

	c.SendFrame(0x240, msg)

	f, err := c.Poll(0x258, 100*time.Millisecond)
	if err != nil {
		log.Println(err)
		return false

	}
	ack[3] = f.Data[0] & 0xBF
	c.SendFrame(0x266, ack)

	seed := int(f.Data[5])<<8 | int(f.Data[6])
	key := calcen(seed, method)

	msgReply[5] = byte(int(key) >> 8 & int(0xFF))
	msgReply[6] = byte(key) & 0xFF

	c.SendFrame(0x240, msgReply)
	f2, err := c.Poll(0x258, 100*time.Millisecond)
	if err != nil {
		log.Println(err)
		return false

	}
	ack[3] = f2.Data[0] & 0xBF
	c.SendFrame(0x266, ack)
	if f2.Data[3] == 0x67 && f2.Data[5] == 0x34 {
		return true
	} else {
		return false
	}
}

func calcen(seed int, method int) int {
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

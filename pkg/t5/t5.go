package t5

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/roffe/gocan"
	"github.com/roffe/gocan/pkg/model"
)

const (
	PBusRate = 615.384
)

type ECUType int

const (
	T52ECU ECUType = iota
	T55ECU16MHZAMDIntel
	T55ECU16MHZCatalyst
	T55ECU20MHZ
	Autodetect
	UnknownECU
	T55ECU
	T55AST52
)

type ECUIdentifier byte

const (
	Partnumber ECUIdentifier = 0x01
	SoftwareID ECUIdentifier = 0x02
	Dataname   ECUIdentifier = 0x03 // SW Version
	EngineType ECUIdentifier = 0x04
	ImmoCode   ECUIdentifier = 0x05
	Unknown    ECUIdentifier = 0x06
	ROMend     ECUIdentifier = 0xFC // Always 07FFFF
	ROMoffset  ECUIdentifier = 0xFD // T5.5 = 040000, T5.2 = 020000
	CodeEnd    ECUIdentifier = 0xFE
)

type Client struct {
	c              *gocan.Client
	defaultTimeout time.Duration
}

func New(c *gocan.Client) *Client {
	t := &Client{
		c:              c,
		defaultTimeout: 250 * time.Millisecond,
	}
	return t
}

func (t *Client) DetermineECU(ctx context.Context) (ECUType, error) {
	footer, err := t.GetECUFooter(ctx)
	if err != nil {
		return UnknownECU, fmt.Errorf("failed to get ECU footer: %v", err)
	}
	chip, err := t.GetChipTypes(ctx)
	if err != nil {
		return UnknownECU, fmt.Errorf("failed to get chiptypes: %v", err)
	}

	flashsize := "256 kB"

	romoffset := getIdentifierFromFooter(footer, ROMoffset)

	switch chip[5] {
	case 0xB8: // Intel/CSI/OnSemi 28F512
	case 0x5D: // Atmel 29C512
	case 0x25: // AMD 28F512
		flashsize = "128 kB"
	case 0xD5: // Atmel 29C010
	case 0xB5: // SST 39F010
	case 0xB4: // Intel/CSI/OnSemi 28F010
	case 0xA7: // AMD 28F010
	case 0xA4: // AMIC 29F010
	case 0x20: // AMD/ST 29F010
	default:
		flashsize = "Unknown"

	}
	var returnECU ECUType
	switch flashsize {
	case "128 kB":
		switch romoffset {
		case "060000":
			log.Println("This is a Trionic 5.2 ECU with 128 kB of FLASH")
			returnECU = T52ECU
		default:
			log.Println("!!! ERROR !!! This is a Trionic 5.2 ECU running an unknown firmware")
			returnECU = UnknownECU
		}
	case "256 kB":
		switch romoffset {
		case "040000":
			log.Println("This is a Trionic 5.5 ECU with 256 kB of FLASH")
			returnECU = T55ECU
		case "060000":
			log.Println("This is a Trionic 5.5 ECU with a T5.2 BIN")
			returnECU = T55AST52
		default:
			log.Println("!!! ERROR !!! This is a Trionic 5.5 ECU running an unknown firmware")
			returnECU = UnknownECU
		}
	}

	log.Println("Part Number:", getIdentifierFromFooter(footer, Partnumber))
	log.Println("Software ID:", getIdentifierFromFooter(footer, SoftwareID))
	log.Println("SW Version:", getIdentifierFromFooter(footer, Dataname))
	log.Println("Engine Type:", getIdentifierFromFooter(footer, EngineType))
	log.Println("IMMO Code:", getIdentifierFromFooter(footer, ImmoCode))
	log.Println("Other Info:", getIdentifierFromFooter(footer, Unknown))
	log.Println("ROM Start: 0x" + romoffset)
	log.Println("Code End: 0x" + getIdentifierFromFooter(footer, CodeEnd))
	log.Println("ROM End: 0x" + getIdentifierFromFooter(footer, ROMend))

	return returnECU, nil
}

func getIdentifierFromFooter(footer []byte, identifier ECUIdentifier) string {

	var result strings.Builder

	offset := len(footer) - 0x05 //  avoid the stored checksum
	for offset > 0 {
		length := int(footer[offset])
		offset--
		search := ECUIdentifier(footer[offset])
		offset--
		if identifier == search {
			for i := 0; i < length; i++ {
				result.WriteByte(footer[offset])
				offset--
			}
			return result.String()
		}
		offset -= length
	}
	log.Printf("error getting identifier %X", identifier)
	return ""

}

func (t *Client) GetChipTypes(ctx context.Context) ([]byte, error) {
	frame := model.NewFrame(0x5, []byte{0xC9, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}, model.ResponseRequired)
	resp, err := t.c.SendAndPoll(ctx, frame, 150*time.Millisecond, 0xC)
	if err != nil {
		return nil, err
	}
	d := resp.Data()
	if d[0] != 0xC9 || d[1] != 0x00 {
		return nil, errors.New("invalid GetChipTypes response")
	}
	return d[2:], nil
}

func (t *Client) GetECUFooter(ctx context.Context) ([]byte, error) {
	log.Println("Getting footer from ECU")
	footer := make([]byte, 0x80)
	var address uint32 = (0x7FF80 + 5)

	for i := 0; i < (0x80 / 6); i++ {
		b, err := t.ReadMemoryByAddress(ctx, address)
		if err != nil {
			return nil, err
		}

		//log.Printf("data: %X", b)
		for j := 0; j < 6; j++ {
			footer[(i*6)+j] = b[j]
		}
		address += 6
	}

	//log.Printf("foot: %X", footer)

	slatten, err := t.ReadMemoryByAddress(ctx, 0x7FFFF)
	if err != nil {
		return nil, err
	}
	//	log.Printf("slatt: %X", slatten)
	for j := 2; j < 6; j++ {
		footer[(0x80-6)+j] = slatten[j]
	}
	//log.Printf("ff: %X", footer)
	return footer, nil
}

func (t *Client) ReadMemoryByAddress(ctx context.Context, address uint32) ([]byte, error) {
	p := []byte{0xC7, byte(address >> 24), byte(address >> 16), byte(address >> 8), byte(address), 0x00, 0x00, 0x00}
	frame := model.NewFrame(0x5, p, model.ResponseRequired)
	resp, err := t.c.SendAndPoll(ctx, frame, 150*time.Millisecond, 0xC)
	if err != nil {
		return nil, err
	}
	data := resp.Data()[2:]
	reverse(data)
	return data, nil
}

func reverse(s []byte) {
	for i, j := 0, len(s)-1; i < j; i, j = i+1, j-1 {
		s[i], s[j] = s[j], s[i]
	}
}

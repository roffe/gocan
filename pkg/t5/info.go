package t5

import (
	"context"
	"errors"
	"log"
)

func (t *Client) DetermineECU(ctx context.Context) (ECUType, error) {
	if !t.bootloaded {
		if err := t.UploadBootLoader(ctx); err != nil {
			return UnknownECU, err
		}
	}

	footer, err := t.GetECUFooter(ctx)
	if err != nil {
		return UnknownECU, err
	}

	chip, err := t.GetChipTypes(ctx)
	if err != nil {
		return UnknownECU, err
	}

	romoffset := GetIdentifierFromFooter(footer, ROMoffset)

	var flashsize uint16
	switch chip[5] {
	case 0xB8, // Intel/CSI/OnSemi 28F512
		0x5D, // Atmel 29C512
		0x25: // AMD 28F512
		flashsize = 128
	case 0xD5, // Atmel 29C010
		0xB5, // SST 39F010
		0xB4, // Intel/CSI/OnSemi 28F010
		0xA7, // AMD 28F010
		0xA4, // AMIC 29F010
		0x20: // AMD/ST 29F010
		flashsize = 256
	default:
		flashsize = 0
	}

	switch flashsize {
	case 128:
		switch romoffset {
		case "060000":
			return T52ECU, nil
		default:
			return UnknownECU, errors.New("!!! ERROR !!! This is a Trionic 5.2 ECU running an unknown firmware")
		}
	case 256:
		switch romoffset {
		case "040000":
			return T55ECU, nil
		case "060000":
			return T55AST52, nil
		default:
			return UnknownECU, errors.New("!!! ERROR !!! This is a Trionic 5.5 ECU running an unknown firmware")
		}
	}

	return UnknownECU, errors.New("!!! ERROR !!! this is a unknown ECU")
}

func (t *Client) PrintECUInfo(ctx context.Context) error {
	footer, err := t.GetECUFooter(ctx)
	if err != nil {
		return err
	}
	log.Println("------------------------------")
	if err := t.printECUType(ctx); err != nil {
		return err
	}
	log.Println("----- ECU info ---------------")
	log.Println("Part Number:  " + GetIdentifierFromFooter(footer, Partnumber))
	log.Println("Software ID:  " + GetIdentifierFromFooter(footer, SoftwareID))
	log.Println("SW Version:   " + GetIdentifierFromFooter(footer, Dataname))
	log.Println("Engine Type:  " + GetIdentifierFromFooter(footer, EngineType))
	log.Println("IMMO Code:    " + GetIdentifierFromFooter(footer, ImmoCode))
	log.Println("Other Info:   " + GetIdentifierFromFooter(footer, Unknown))
	log.Println("ROM Start:    0x" + GetIdentifierFromFooter(footer, ROMoffset))
	log.Println("Code End:     0x" + GetIdentifierFromFooter(footer, CodeEnd))
	log.Println("ROM End:      0x" + GetIdentifierFromFooter(footer, ROMend))
	log.Println("------------------------------")
	return nil
}

func (t *Client) printECUType(ctx context.Context) error {
	typ, err := t.DetermineECU(ctx)
	if err != nil {
		return err
	}
	switch typ {
	case T52ECU:
		log.Println("This is a Trionic 5.2 ECU with 128 kB of FLASH")
	case T55AST52:
		log.Println("This is a Trionic 5.5 ECU with a T5.2 BIN")
	case T55ECU:
		log.Println("This is a Trionic 5.5 ECU with 256 kB of FLASH")
	default:
		return errors.New("printECUType: unknown ECU")
	}
	return nil
}

package t5

import (
	"context"
	"errors"
	"log"

	"github.com/roffe/gocan/pkg/model"
)

func (t *Client) Info(ctx context.Context, callback model.ProgressCallback) ([]model.HeaderResult, error) {
	if !t.bootloaded {
		if err := t.UploadBootLoader(ctx, callback); err != nil {
			return nil, err
		}
	}
	footer, err := t.GetECUFooter(ctx)
	if err != nil {
		return nil, err
	}

	var out []model.HeaderResult
	for _, d := range T5Headers {
		h := GetIdentifierFromFooter(footer, d.ID)
		a := model.HeaderResult{Value: h}
		a.Desc = d.Desc
		a.ID = d.ID
		out = append(out, a)
	}
	return out, nil
}

func (t *Client) PrintECUInfo(ctx context.Context) error {
	res, err := t.Info(ctx, nil)
	if err != nil {
		return err
	}

	log.Println("----- ECU info ---------------")
	if err := t.printECUType(ctx); err != nil {
		return err
	}

	for _, r := range res {
		log.Println(r.Desc, r.Value)
	}
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

func (t *Client) DetermineECU(ctx context.Context) (ECUType, error) {
	//if !t.bootloaded {
	//	if err := t.UploadBootLoader(ctx); err != nil {
	//		return UnknownECU, err
	//	}
	//}

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

var T5Headers = []model.Header{
	{Desc: "Part Number", ID: 0x01},
	{Desc: "Software ID", ID: 0x02},
	{Desc: "SW Version", ID: 0x03},
	{Desc: "Engine Type", ID: 0x04},
	{Desc: "IMMO Code", ID: 0x05},
	{Desc: "Other Info", ID: 0x06},
	{Desc: "ROM Start", ID: 0xFD},
	{Desc: "Code End", ID: 0xFC},
	{Desc: "ROM End", ID: 0xFE},
}

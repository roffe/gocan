package t7

import (
	"fmt"
	"log"
)

type BinInfo struct {
	Vin             string // 0x90
	HwPartNo        string // 0x91
	ImmoCode        string //0x92
	SoftwarePartNo  string //0x94
	SoftwareVersion string // 0x95
	EngineType      string // 0x97
	Tester          string // 0x98
	SoftwareDate    string // 0x99
}

func GetBinInfo(bin []byte) BinInfo {
	vin, err := GetHeaderField(bin, 0x90)
	if err != nil {
		log.Println(err)
	}
	hw, err := GetHeaderField(bin, 0x91)
	if err != nil {
		log.Println(err)
	}
	immo, err := GetHeaderField(bin, 0x92)
	if err != nil {
		log.Println(err)
	}
	spno, err := GetHeaderField(bin, 0x94)
	if err != nil {
		log.Println(err)
	}
	sver, err := GetHeaderField(bin, 0x95)
	if err != nil {
		log.Println(err)
	}
	etype, err := GetHeaderField(bin, 0x97)
	if err != nil {
		log.Println(err)
	}
	tester, err := GetHeaderField(bin, 0x98)
	if err != nil {
		log.Println(err)
	}
	swdate, err := GetHeaderField(bin, 0x99)
	if err != nil {
		log.Println(err)
	}
	return BinInfo{
		vin, hw, immo, spno, sver, etype, tester, swdate,
	}

}

func GetHeaderField(bin []byte, id byte) (string, error) {
	binLength := len(bin)
	var answer []byte
	addr := binLength - 1
	var found bool
	for addr > (binLength - 0x1FF) {
		/* The first byte is the length of the data */
		fieldLength := bin[addr]
		//log.Printf("%3d, %x", lengthField, lengthField)
		if fieldLength == 0x00 || fieldLength == 0xFF {
			break
		}
		addr--

		/* Second byte is an ID field */
		fieldID := bin[addr]
		addr--

		if fieldID == id {
			answer = make([]byte, int(fieldLength))
			answer[fieldLength-1] = 0x00
			//answer[fieldLength] = 0x00
			for i := 0; i < int(fieldLength); i++ {
				answer[i] = bin[addr]
				addr--
			}
			log.Printf("0x%02x %d> %q", fieldID, len(answer), string(answer))
			found = true
			//break
			// when this return is commented out, the function will
			// find the last field if there are several (mainly this
			// is for searching for the last VIN field)
			// return 1;
		}
		addr -= int(fieldLength)

	}
	if found {
		return string(answer), nil
	}
	return "", fmt.Errorf("did not find header for id 0x%02x", id)
}

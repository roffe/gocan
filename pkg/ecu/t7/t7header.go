package t7

import (
	"fmt"
	"io"
	"log"
	"os"
)

type FileHeaderField struct {
	ID     byte
	Length int
	Data   []byte
}

func (f *FileHeaderField) SetString(v string) {
	if len([]byte(v)) > int(f.Length) {
		panic("to big")
	}
	f.Data = []byte(v)
}

func (f *FileHeaderField) String() string {
	return string(f.Data[:])
}

func smallintBytes(n int) []byte {
	b := []byte{byte(n >> 8), byte(n)}
	return b
}

func intBytes(n int) []byte {

	b := []byte{byte(n >> 24), byte(n >> 16), byte(n >> 8), byte(n)}
	return b
}

func (f *FileHeaderField) SetInt(n int) {
	b := make([]byte, 4)
	b[3] = byte(n)
	n >>= 8
	b[2] = byte(n)
	n >>= 8
	b[1] = byte(n)
	n >>= 8
	b[0] = byte(n)
	f.Data = b
}

func (f *FileHeaderField) Int() int {
	var val int
	val |= int(f.Data[0])
	val <<= 8
	val |= int(f.Data[1])
	val <<= 8
	val |= int(f.Data[2])
	val <<= 8
	val |= int(f.Data[3])
	return val
}

func (f *FileHeaderField) SmallInt() int {
	var val int
	val |= int(f.Data[0])
	val <<= 8
	val |= int(f.Data[1])
	return val
}

func (f *FileHeaderField) Byte() byte {
	var b byte = 0
	b |= f.Data[0]
	return b
}

func (f *FileHeaderField) Date() [5]byte {
	var ret [5]byte
	for i := 0; i < len(f.Data); i++ {
		ret[i] = f.Data[i]
	}
	return ret
}

func (f *FileHeaderField) Pretty() string {
	return fmt.Sprintf("ID: %02X, Length: %d, Data: %q", f.ID, f.Length, f.Data)
}

func ReadField(file io.ReadWriteSeeker) (*FileHeaderField, error) {
	sizeb := make([]byte, 1)
	file.Read(sizeb)

	file.Seek(-2, io.SeekCurrent)

	idb := make([]byte, 1)
	file.Read(idb)

	if idb[0] == 0xFF {
		return &FileHeaderField{
			ID:     0xFF,
			Length: 0,
		}, nil
	}

	size := int64(sizeb[0])

	data := make([]byte, size)

	file.Seek(-(size + 1), io.SeekCurrent)

	file.Read(data)

	file.Seek(-(size + 1), io.SeekCurrent)
	fhf := &FileHeaderField{
		ID:     idb[0],
		Length: int(sizeb[0]),
		Data:   reverse(data),
	}
	return fhf, nil
}

/// FileHeader represents the header (or, rather, tailer) of a T7 firmware file.
/// The header contains meta data that describes some important parts of the firmware.
///
/// The header consists of several fields here represented by the FileHeaderField class.

type FileHeader struct {
	chassisIDCounter            int
	chassisIDDetected           bool
	immoCodeDetected            bool
	symbolTableMarkerDetected   bool
	symbolTableChecksumDetected bool
	f2ChecksumDetected          bool

	chassisID          string
	immobilizerID      string
	romChecksumType    int
	bottomOfFlash      int
	romChecksumError   byte
	valueF5            int
	valueF6            int
	valueF7            int
	valueF8            int
	value9C            int
	symbolTableAddress int
	vehicleIDnr        string
	dateModified       string
	lastModifiedBy     [5]byte
	testSerialNr       string
	engineType         string
	ecuHardwareNr      string
	softwareVersion    string
	carDescription     string
	partNumber         string
	checksumF2         int
	checksumFB         int
	fwLength           int
}

func (f *FileHeader) GetVin() string {
	return f.chassisID
}

func (f *FileHeader) SetVin(vin string) {
	if len(vin) > 17 {
		panic("VIN to long")
	}
	f.chassisID = vin
}

func NewFileHeader(filename string, autoFixFooter bool) (*FileHeader, error) {
	file, err := os.OpenFile(filename, os.O_RDWR, 0666)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	fh := new(FileHeader)

	// init new values
	fh.chassisID = "00000000000000000"
	fh.immobilizerID = "000000000000000"
	fh.engineType = "0000000000000"
	fh.vehicleIDnr = "000000000"
	fh.partNumber = "0000000"
	fh.softwareVersion = "000000000000"
	fh.carDescription = "00000000000000000000"
	fh.dateModified = "0000"
	fh.ecuHardwareNr = "0000000"
	fh.SetLastModifiedBy(0x42, 0)
	fh.SetLastModifiedBy(0xFB, 1)
	fh.SetLastModifiedBy(0xFA, 2)
	fh.SetLastModifiedBy(0xFF, 3)
	fh.SetLastModifiedBy(0xFF, 4)
	fh.testSerialNr = "050225"

	if _, err := file.Seek(-1, io.SeekEnd); err != nil {
		return nil, err
	}

	for {
		fhf, err := ReadField(file)
		if err != nil {
			return nil, err
		}
		if fhf.ID == 0xFF || fhf.ID == 0x00 {
			break
		}
		fh.parseField(fhf)
	}

	if (fh.chassisIDCounter > 1 || !fh.immoCodeDetected || !fh.chassisIDDetected) && autoFixFooter {
		log.Println("bad footer detected & auto fix enabled")
		fh.clearFooter(file)
		fh.createNewFooter(file, fh.symbolTableMarkerDetected, fh.symbolTableChecksumDetected, fh.f2ChecksumDetected)
	}

	log.Printf("%+v", fh)

	return fh, nil
}
func (fh *FileHeader) parseField(fhf *FileHeaderField) {
	switch fhf.ID {
	case 0x90:
		fh.chassisID = fhf.String()
		fh.chassisIDDetected = true
		fh.chassisIDCounter++
	case 0x91:
		fh.vehicleIDnr = fhf.String()
	case 0x92:
		fh.immobilizerID = fhf.String()
		fh.immoCodeDetected = true
	case 0x93:
		fh.ecuHardwareNr = fhf.String()
	case 0x94:
		fh.partNumber = fhf.String()
	case 0x95:
		fh.softwareVersion = fhf.String()
	case 0x97:
		fh.carDescription = fhf.String()
	case 0x98:
		fh.engineType = fhf.String()
	case 0x99:
		fh.testSerialNr = fhf.String()
	case 0x9A:
		fh.dateModified = fhf.String()
	case 0x9B:
		fh.symbolTableAddress = fhf.Int()
		fh.symbolTableMarkerDetected = true
	case 0x9C:
		fh.value9C = fhf.Int()
		fh.symbolTableChecksumDetected = true
	case 0xF2:
		fh.checksumF2 = fhf.Int()
		fh.f2ChecksumDetected = true
	case 0xF5:
		fh.valueF5 = fhf.SmallInt()
	case 0xF6:
		fh.valueF6 = fhf.SmallInt()
	case 0xF7:
		fh.valueF7 = fhf.SmallInt()
	case 0xF8:
		fh.valueF8 = fhf.SmallInt()
	case 0xF9:
		fh.romChecksumError = fhf.Byte()
	case 0xFA:
		fh.lastModifiedBy = fhf.Date()
	case 0xFB:
		fh.checksumFB = fhf.Int()
	case 0xFC:
		fh.bottomOfFlash = fhf.Int()
	case 0xFD:
		fh.romChecksumType = fhf.Int()
	case 0xFE:
		fh.fwLength = fhf.Int()
	default:
		panic(fmt.Sprintf("Unknown ID: 0x%02X", fhf.ID))
	}
}

func (f *FileHeader) clearFooter(file io.ReadWriteSeeker) {
	log.Println("clear footer")
	end, _ := file.Seek(0, io.SeekEnd)
	file.Seek(0x07FE00, io.SeekStart)
	length := end - 0x7FE00

	piArea := make([]byte, length)
	piArea[0] = 0xFF
	for j := 1; j < len(piArea); j *= 2 {
		copy(piArea[j:], piArea[:j])
	}

	n, err := file.Write(piArea)
	if err != nil {
		log.Fatal(err)
	}

	if n != int(length) {
		log.Println("dgaf what i wrote")
	}

	/*
		for i := 0; i < int(length); i++ {
			_, err := file.Write([]byte{0xFF})
			if err != nil {
				if err == io.EOF {
					break
				}
				log.Fatal(err)
			}
		}
	*/
}

func (f *FileHeader) createNewFooter(file io.ReadWriteSeeker, create9B bool, create9C bool, createF2 bool) {
	log.Println("write footer")
	headers := []FileHeaderField{
		{
			ID:     0x91,
			Length: len(f.vehicleIDnr),
			Data:   []byte(f.vehicleIDnr),
		},
		{
			ID:     0x94,
			Length: len(f.partNumber),
			Data:   []byte(f.partNumber),
		},
		{
			ID:     0x95,
			Length: len(f.softwareVersion),
			Data:   []byte(f.softwareVersion),
		},
		{
			ID:     0x97,
			Length: len(f.carDescription),
			Data:   []byte(f.carDescription),
		},
		{
			ID:     0x9A,
			Length: len(f.dateModified),
			Data:   []byte(f.dateModified),
		},
	}
	part2 := []FileHeaderField{
		{
			ID:     0xFB,
			Length: 4,
			Data:   intBytes(f.checksumFB),
		},
		{
			ID:     0xFC,
			Length: 4,
			Data:   intBytes(f.bottomOfFlash),
		},
		{
			ID:     0xFD,
			Length: 4,
			Data:   intBytes(f.romChecksumType),
		},
		{
			ID:     0xFE,
			Length: 4,
			Data:   intBytes(f.fwLength),
		},
		{
			ID:     0xFA,
			Length: 5,
			Data:   f.lastModifiedBy[:],
		},
		{
			ID:     0x92,
			Length: len(f.immobilizerID),
			Data:   []byte(f.immobilizerID),
		},
		{
			ID:     0x93,
			Length: len(f.ecuHardwareNr),
			Data:   []byte(f.ecuHardwareNr),
		},
		{
			ID:     0xF8,
			Length: 2,
			Data:   smallintBytes(f.valueF8),
		},
		{
			ID:     0xF7,
			Length: 2,
			Data:   smallintBytes(f.valueF7),
		},
		{
			ID:     0xF6,
			Length: 2,
			Data:   smallintBytes(f.valueF6),
		},
		{
			ID:     0xF5,
			Length: 2,
			Data:   smallintBytes(f.valueF5),
		},
		{
			ID:     0x90,
			Length: len(f.chassisID),
			Data:   []byte(f.chassisID),
		},
		{
			ID:     0x99,
			Length: len(f.testSerialNr),
			Data:   []byte(f.testSerialNr),
		},
		{
			ID:     0x98,
			Length: len(f.engineType),
			Data:   []byte(f.engineType),
		},
		{
			ID:     0xF9,
			Length: 1,
			Data:   []byte{0x00},
		},
	}

	if create9C {
		h9c := FileHeaderField{
			ID:     0x9C,
			Length: 4,
			Data:   intBytes(f.value9C),
		}
		headers = append(headers, h9c)
	}

	if create9B {
		h9b := FileHeaderField{
			ID:     0x9B,
			Length: 4,
			Data:   intBytes(f.symbolTableAddress),
		}
		headers = append(headers, h9b)
	}

	if createF2 {
		f2 := FileHeaderField{
			ID:     0xF2,
			Length: 4,
			Data:   intBytes(f.checksumF2),
		}
		headers = append(headers, f2)
	}

	headers = append(headers, part2...)

	file.Seek(-1, io.SeekEnd)
	for _, h := range headers {
		f.writeField(file, h)
	}

}

func (fh *FileHeader) writeField(file io.ReadWriteSeeker, f FileHeaderField) {
	log.Println(f.Pretty())
	// Write length
	file.Write([]byte{byte(f.Length)})
	file.Seek(-2, io.SeekCurrent)

	// Write ID
	file.Write([]byte{f.ID})
	file.Seek(-2, io.SeekCurrent) // Skip ID and length

	for i := 0; i < f.Length; i++ {
		file.Write([]byte{f.Data[i]})
		file.Seek(-2, io.SeekCurrent)
	}
}

func (fh *FileHeader) SetLastModifiedBy(value byte, pos int) {
	fh.lastModifiedBy[pos] = value
}

func reverse(s []byte) []byte {
	for i, j := 0, len(s)-1; i < j; i, j = i+1, j-1 {
		s[i], s[j] = s[j], s[i]
	}
	return s
}

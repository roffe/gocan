package t7

import (
	"fmt"
	"io"
	"log"
)

/// FileHeader represents the header (or, rather, tailer) of a T7 firmware file.
/// The header contains meta data that describes some important parts of the firmware.
///
/// The header consists of several fields here represented by the FileHeaderField class.

type FileHeader struct {
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

type FileHeaderField struct {
	ID     byte
	Length byte
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

func (f *FileHeaderField) Pretty() string {
	return fmt.Sprintf("ID: %02X, Length: %d, Data: %q", f.ID, f.Length, f.Data)
}

func ReadField(file io.ReadSeeker) (*FileHeaderField, error) {
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
		Length: sizeb[0],
		Data:   reverse(data),
	}
	return fhf, nil
}

func NewFileHeader(file io.ReadSeeker) (*FileHeader, error) {
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

	_, err := file.Seek(-1, io.SeekEnd)
	if err != nil {
		return nil, err
	}

outer:
	for {
		file.Seek(0, io.SeekCurrent)
		fhf, err := ReadField(file)
		if err != nil {
			return nil, err
		}
		switch fhf.ID {
		case 0x90:
			fh.chassisID = fhf.String()
		case 0x91:
			fh.vehicleIDnr = fhf.String()
		case 0x92:
			fh.immobilizerID = fhf.String()
			log.Println(fhf.Pretty())
		case 0xFF:
			break outer
		}
	}

	return fh, nil
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

package srec

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"log"
	"strconv"
)

// constructed values of a field that always has a fixed length in the S record
const (
	TypeLen         = 1
	LengthLen       = 1
	S0AddrLen       = 2
	S1AddrLen       = 2
	S2AddrLen       = 3
	S3AddrLen       = 4
	S5AddrLen       = 0
	S7AddrLen       = 4
	S8AddrLen       = 3
	S9AddrLen       = 2
	CSumLen         = 1
	maxSizeOfS1Addr = 0xFFFF
	maxSizeOfS2Addr = 0xFFFFFF
	maxSizeOfS3Addr = 0xFFFFFFFF
)

// Srec is a type of structure with multiple Record structures
type Srec struct {
	Records []*Record
}

// Record is a type of structure that expresses S records per row
type Record struct {
	Srectype string
	Length   uint8
	Address  uint32
	Data     []byte
	Checksum uint8
}

// NewSrec returns a new Srec object
func NewSrec() *Srec {
	return &Srec{}
}

// newRecord returns a new Srec object
func newRecord() *Record {
	return &Record{}
}

// getAddrLen returns the length of address for each srectype as byte length
// S4, S6 are not handled
func getAddrLen(srectype string) (int, error) {
	switch srectype {
	case "S0":
		return S0AddrLen, nil
	case "S1":
		return S1AddrLen, nil
	case "S2":
		return S2AddrLen, nil
	case "S3":
		return S3AddrLen, nil
	case "S5":
		return S5AddrLen, nil
	case "S7":
		return S7AddrLen, nil
	case "S8":
		return S8AddrLen, nil
	case "S9":
		return S9AddrLen, nil
	default:
		return 0, fmt.Errorf("%s is not srectype", srectype)
	}
}

// EndAddress returns endaddress of the record
func (rec *Record) EndAddress() uint32 {
	return rec.Address + uint32(len(rec.Data)) - 1
}

// CalcChecksum calculates the checksum value of the record from the information of the arguments
func (rec *Record) CalcChecksum() (uint8, error) {
	return calcChecksum(rec.Srectype, rec.Length, rec.Address, rec.Data)
}

// CalcChecksum calculates the checksum value of the record from the information of the arguments
// S4, S6 are not handled
func calcChecksum(srectype string, len uint8, addr uint32, data []byte) (uint8, error) {
	bytes := []byte{len}
	switch srectype {
	case "S0", "S1", "S9":
		bytes = append(bytes, byte((addr&0xFF00)>>8), byte((addr&0x00FF)>>0))
	case "S2", "S8":
		bytes = append(bytes, byte((addr&0xFF0000)>>16), byte((addr&0x00FF00)>>8), byte((addr&0x0000FF)>>0))
	case "S3", "S7":
		bytes = append(bytes, byte((addr&0xFF000000)>>24), byte((addr&0x00FF0000)>>16), byte((addr&0x0000FF00)>>8), byte((addr&0x000000FF))>>0)
	case "S5":
		// Since S5 has no address part, it is not added to bytes
	default:
		return 0, fmt.Errorf("%s is invalid srectype", srectype)
	}
	bytes = append(bytes, data...)
	sum := byte(0)
	for _, b := range bytes {
		sum += b
	}
	return uint8(^sum), nil
}

// Parse creates a Srec object (basically an opened file) from io.Reader
// If it contains a character string that starts with S4, S6 or does not start S [0-9] to the first 2 bytes,
// processing stops and an error will be return
func (srs *Srec) Parse(fileReader io.Reader) error {
	scanner := bufio.NewScanner(fileReader)
	scanner.Split(scanLinesCustom)

	for scanner.Scan() {
		line := scanner.Text()

		srectype := line[:2]
		switch srectype {
		case "S0", "S1", "S2", "S3", "S5", "S7", "S8", "S9":
			rec := newRecord()
			err := rec.getRecordFields(line)
			if err != nil {
				return err
			}
			srs.Records = append(srs.Records, rec)
		default:
			return fmt.Errorf("%s is invalid srectype", srectype)
		}
	}
	return nil
}

// getRecordFields creates a Record object from a string
func (rec *Record) getRecordFields(line string) error {
	// srectype
	stype := line[:2]
	// Acquire the length of the field of address according to srectype
	addrLen, err := getAddrLen(line[:2])
	if err != nil {
		return err
	}
	dataLen, err := strconv.ParseUint(line[2:4], 16, 32)
	if err != nil {
		return err
	}
	// address
	// Since there is no address part in S5, leave the initial value at 0 (since null character does not become 0 in ParseUint)
	addr := uint64(0)
	if stype != "S5" {
		addr, err = strconv.ParseUint(line[4:4+addrLen*2], 16, 32)
		if err != nil {
			return err
		}
	}
	// data
	data := make([]byte, 0)
	dataIndexSt := TypeLen*2 + LengthLen*2 + addrLen*2
	dataIndexEd := (TypeLen*2 + LengthLen*2) + (int(dataLen)*2 - CSumLen*2)
	for i := dataIndexSt; i < dataIndexEd; i += 2 {
		var b uint64
		b, err = strconv.ParseUint(line[i:i+2], 16, 32)
		if err != nil {
			return err
		}
		data = append(data, byte(b))
	}
	// checksum
	cSumIndexSt := TypeLen*2 + LengthLen*2 + dataLen*2 - CSumLen*2
	cSumIndexEd := TypeLen*2 + LengthLen*2 + dataLen*2
	csum, err := strconv.ParseUint(line[cSumIndexSt:cSumIndexEd], 16, 32)
	if err != nil {
		return err
	}
	rec.Srectype = stype
	rec.Length = uint8(dataLen)
	rec.Address = uint32(addr)
	rec.Data = data
	rec.Checksum = uint8(csum)
	return nil
}

// EndAddr returns the record's end address from the records of the Srec object
// Since the end address of the S record is (address of the last record + data length -1), 1 is subtracted
// Please be aware that processing costs are high as it traverses the entire object
func (srs *Srec) EndAddr() uint32 {
	if len(srs.Records) == 0 {
		return 0
	}
	max := uint32(0x00000000)
	dLen := uint32(0)
	for _, r := range srs.Records {
		switch r.Srectype {
		case "S1", "S2", "S3":
			if r.Address > max {
				max = r.Address
				dLen = uint32(len(r.Data))
			}
		}
	}
	return max + dLen - 1
}

// MakeRec creates and returns a new Record object from the argument information
func MakeRec(srectype string, addr uint32, data []byte) (*Record, error) {
	r := newRecord()
	r.Srectype = srectype
	// Since EndAddress is calculated based on Address and Data, it is set before checking whether to convert
	r.Address = addr
	r.Data = data
	switch r.Srectype {
	case "S1", "S2":
		if r.EndAddress() > maxSizeOfS1Addr {
			r.Srectype = "S2"
		}
		if r.EndAddress() > maxSizeOfS2Addr {
			r.Srectype = "S3"
		}
	default:
	}
	// Checksum, Length operation depends on srectype, so calculate them after it is fixed
	l, err := getAddrLen(r.Srectype)
	if err != nil {
		return nil, err
	}
	r.Length = uint8(l) + uint8(len(data)) + CSumLen
	r.Checksum, err = calcChecksum(r.Srectype, r.Length, r.Address, r.Data)
	if err != nil {
		return nil, err
	}
	return r, nil
}

// scanLinesCustom is a splitFunc corresponding to all line feed codes of CRLF, LF, CR
func scanLinesCustom(data []byte, atEOF bool) (advance int, token []byte, err error) {
	if atEOF && len(data) == 0 {
		return 0, nil, nil
	}
	i, j := bytes.IndexByte(data, '\n'), bytes.IndexByte(data, '\r')
	if i < j {
		// if LF
		if i >= 0 {
			return i + 1, data[0:i], nil
		}
		// if CRLF
		if j < len(data)-1 && isLF(data[j+1]) {
			return j + 2, data[0:j], nil
		}
		// if CR
		return j + 1, data[0:j], nil
	} else if j < i {
		if j >= 0 {
			// if CRLF
			if j < len(data)-1 && isLF(data[j+1]) {
				return j + 2, data[0:j], nil
			}
			// if CR
			return j + 1, data[0:j], nil
		}
		// if LF
		return i + 1, data[0:i], nil
	} //else {
	// this case is only "i == -1 && j == -1"
	//	}
	// If we're at EOF, we have a final, non-terminated line. Return it.
	if atEOF {
		return len(data), data, nil
	}
	// Request more data.
	return 0, nil, nil
}

// isLF returns weather b is LF
func isLF(b byte) bool {
	return b == '\n'
}

// String return the contents of the entire Srecord.
func (sr *Srec) String() string {
	fs := ""
	for _, r := range sr.Records {
		fs += r.String() + "\n"
	}
	return fs
}

// String returns the contents of the Record. If the Record has wrong srectype,
// it returns "<invalid srectype>"
func (r *Record) String() string {
	fs := ""
	switch r.Srectype {
	case "S0", "S1", "S9":
		fs = fmt.Sprintf("%s%02X%04X", r.Srectype, r.Length, r.Address)
	case "S2", "S8":
		fs = fmt.Sprintf("%s%02X%06X", r.Srectype, r.Length, r.Address)
	case "S3", "S7":
		fs = fmt.Sprintf("%s%02X%08X", r.Srectype, r.Length, r.Address)
	case "S5":
		// Since S5 has no address part, it does not format it
		fs = fmt.Sprintf("%s%02X", r.Srectype, r.Length)
	default:
		return "<invalid srectype>"
	}
	for _, b := range r.Data {
		fs += fmt.Sprintf("%02X", b)
	}
	fs += fmt.Sprintf("%02X", r.Checksum)
	return fs
}

func printBytes(bt []byte) {
	for i, b := range bt {
		if i != 0 && i%16 == 0 {
			log.Println()
		}
		log.Printf("%02X", b)
	}
	log.Print("\n")
}

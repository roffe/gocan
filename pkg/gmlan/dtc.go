package gmlan

type DTC struct {
	Code   string
	Status byte
}

// How to read DTC codes
//B0 B1    First DTC character
//-- --    -------------------
// 0  0    P - Powertrain
// 0  1    C - Chassis
// 1  0    B - Body
// 1  1    U - Network

//B2 B3    Second DTC character
//-- --    --------------------
// 0  0    0
// 0  1    1
// 1  0    2
// 1  1    3

//B4 B5 B6 B7    Third/Fourth/Fifth DTC characters
//-- -- -- --    -------------------
// 0  0  0  0    0
// 0  0  0  1    1
// 0  0  1  0    2
// 0  0  1  1    3
// 0  1  0  0    4
// 0  1  0  1    5
// 0  1  1  0    6
// 0  1  1  1    7
// 1  0  0  0    8
// 1  0  0  1    9
// 1  0  1  0    A
// 1  0  1  1    B
// 1  1  0  0    C
// 1  1  0  1    D
// 1  1  1  0    E
// 1  1  1  1    F

// Example
// E1 03 ->
// 1110 0001 0000 0011
// 11=U
//
//	10=2
//	   0001=1
//	        0000=0
//	             0011=3
//
// ----------------------
// U2103
// DecodeDTC decodes a 2-byte DTC value (A,B) into a string like "P0122".
// Returns "" if both bytes are zero (often means "no code").
func DecodeDTC(a, b byte) string {
	if a == 0 && b == 0 {
		return ""
	}

	systemChars := [4]byte{'P', 'C', 'B', 'U'}
	secondDigit := [4]byte{'0', '1', '2', '3'}
	hexDigits := "0123456789ABCDEF"

	code := make([]byte, 5)

	// A7..A6 -> system letter (P/C/B/U)
	code[0] = systemChars[(a>>6)&0x03]

	// A5..A4 -> 2nd digit (0..3)
	code[1] = secondDigit[(a>>4)&0x03]

	// A3..A0 -> 3rd digit (0..F)
	code[2] = hexDigits[a&0x0F]

	// B7..B4 -> 4th digit (0..F)
	code[3] = hexDigits[(b>>4)&0x0F]

	// B3..B0 -> 5th digit (0..F)
	code[4] = hexDigits[b&0x0F]

	return string(code)
}

// DecodeDTCSlice expects at least two bytes and decodes the first two into a DTC string.
// Returns "" if the slice is too short or the DTC is zero.
func DecodeDTCSlice(data []byte) string {
	if len(data) < 2 {
		return ""
	}
	return DecodeDTC(data[0], data[1])
}

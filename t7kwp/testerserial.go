package t7kwp

// Trionic 7 tester-serial kill-list.
//
// XEolprg.c, END_EOL state, writeDataByLocalIdentifier 0x98:
//
//	else if( CheckTesterSerialNumber( &p_recBuffer[3] ) )
//	{
//	   FlashErase(0);            // wipe the whole chip
//	   TurnOffSystem = 0;
//	   TurnOffSystem();          // jump to address 0
//	}
//
// CheckTesterSerialNumber lives in TESTLIM.OBJ (Build/TESTLIM.OBJ in the
// eu03_erik_org tree). Its 68k code packs Name[9..11] into a 16-bit code and
// compares it against a 26-entry table: a MATCH bricks the ECU, everything else
// is accepted. So this is a block-list, not an allow-list.
//
// Verified against the object file: the table sits at TESTLIM.OBJ+0xD18 and the
// routine at +0xD88 (link a6,#-0x68 / 26× move.l (a1)+,(a0)+ / the three
// cmpi.b #0x20 branches below / dbf-style scan / moveq #1 on match).
var testerSerialKillList = [26]uint16{
	0x4232, 0x4235, 0x4236, 0x4340, 0x4342, 0x4349, 0x4353, 0x4365, 0x4376,
	0x4381, 0x4392, 0x43A1, 0x6553, 0x6557, 0x6561, 0x65A9, 0x65B2, 0x65B4,
	0x6751, 0x6770, 0x6774, 0x6780, 0x67A2, 0x67B1, 0x69C3, 0x7388,
}

// packTesterSerial reproduces the firmware's 16-bit packing of Name[9..11].
// Real serials look like "PPCAN Nr 592": a fixed 9-char prefix then 1-3 digits,
// which is why only those three offsets matter.
func packTesterSerial(name []byte) uint16 {
	switch {
	case name[10] == 0x20: // "d  " — one significant char
		return 0x4200 + uint16(name[9])
	case name[11] == 0x20: // "dd " — two significant chars
		return 0x4000 + uint16(name[9])<<4 + uint16(name[10])
	default: // "ddd"
		return uint16(name[9])<<9 + uint16(name[10])<<4 + uint16(name[11])
	}
}

// PadTesterSerial space-pads a tester serial to 13 bytes, the width the ECU's
// buffer expects. Shorter values leave offsets 9..11 pointing at whatever the
// diagnostic buffer still held, which is exactly the input TesterSerialBlocked
// has to reason about — so pad first, then check, then send the padded value.
func PadTesterSerial(serial string) []byte {
	const width = 13
	out := make([]byte, width)
	for i := range out {
		if i < len(serial) {
			out[i] = serial[i]
			continue
		}
		out[i] = 0x20
	}
	return out
}

// TesterSerialBlocked reports whether writing serial as PI-area field 0x98 would
// make the ECU erase its own flash and halt — recoverable only over BDM.
// Never send a 0x98 that this returns true for.
func TesterSerialBlocked(serial []byte) bool {
	if len(serial) < 12 {
		return true // the ECU would read past the value; refuse rather than guess
	}
	code := packTesterSerial(serial)
	for _, k := range testerSerialKillList {
		if k == code {
			return true
		}
	}
	return false
}

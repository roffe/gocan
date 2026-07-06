package gmlan

import "testing"

func TestFailureTypeString(t *testing.T) {
	for b, want := range map[byte]string{
		0x02: "short to ground",
		0x05: "short to battery or open",
		0x36: "EEPROM failure",
		0x72: "alive counter incorrect/not updated",
		0x10: "reserved",        // enumerated as reserved -> fallback
		0x8A: "reserved",        // category reserved by document
		0xF3: "system specific", // category F
	} {
		if got := FailureTypeString(b); got != want {
			t.Errorf("FailureTypeString(%#02x) = %q, want %q", b, got, want)
		}
	}
}

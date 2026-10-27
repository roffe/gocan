package t7kwp

import (
	"context"
	"testing"
)

// Cases carried over from a reference reimplementation of the ECU's check.
func TestTesterSerialBlocked(t *testing.T) {
	for _, tt := range []struct {
		serial  string
		blocked bool
	}{
		{"PPCAN Nr 1", false},    // 0x4231
		{"PPCAN Nr 2", true},     // 0x4232
		{"PPCAN Nr 5", true},     // 0x4235
		{"PPCAN Nr 592", false},  // 0x6DC2, seen in a real bin
		{"PPCAN Nr 858", true},   // 0x7388, three digits
		{"PPCAN Nr 71", true},    // 0x43A1, two digits
		{"0000000000000", false}, // 0x6330, the factory default
		{"SAAB_PROD_EOL", false},
	} {
		if got := TesterSerialBlocked(PadTesterSerial(tt.serial)); got != tt.blocked {
			t.Errorf("TesterSerialBlocked(%q) = %v, want %v", tt.serial, got, tt.blocked)
		}
	}

	if !TesterSerialBlocked([]byte("short")) {
		t.Error("a serial too short to reach offset 11 must be refused")
	}

	if err := new(Client).WriteDataByLocalIdentifier(context.Background(), 0x98, PadTesterSerial("PPCAN Nr 2")); err == nil {
		t.Error("WriteDataByLocalIdentifier sent a kill-listed tester serial")
	}
}

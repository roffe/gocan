package t7kwp

import (
	"testing"
)

// oldCalcen is the pre-refactor int-with-explicit-masks implementation. The
// uint16 version in calcKey must match it for every seed — a wrong key just
// gets securityAccessDenied from the ECU, with no hint that the math drifted.
func oldCalcen(seed, xor, sub int) int {
	key := seed << 2
	key &= 0xFFFF
	key ^= xor
	key -= sub
	key &= 0xFFFF
	return key
}

func TestSeedKeyMatchesOldCalcen(t *testing.T) {
	keys := append([]SeedKey{{XOR: 0x1234, Sub: 0x5678}}, KnownSeedKeys...)
	for _, k := range keys {
		for s := 0; s <= 0xFFFF; s++ {
			got := calcKey(uint16(s), k)
			want := uint16(oldCalcen(s, int(k.XOR), int(k.Sub)))
			if got != want {
				t.Fatalf("XOR %04X SUB %04X seed %04X: got %04X, want %04X", k.XOR, k.Sub, s, got, want)
			}
		}
	}
}

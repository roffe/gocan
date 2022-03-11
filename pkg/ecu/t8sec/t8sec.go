package t8sec

func CalculateAccessKey(seed []byte, level byte) (byte, byte) {
	val := int(seed[0])<<8 | int(seed[1])

	key := convertSeed(val)

	switch level {
	case 0xFB:
		key ^= 0x8749
		key += 0x06D3
		key ^= 0xCFDF
	case 0xFD:
		key /= 3
		key ^= 0x8749
		key += 0x0ACF
		key ^= 0x81BF
	}

	return (byte)((key >> 8) & 0xFF), (byte)(key & 0xFF)
}

func convertSeed(seed int) int {
	key := seed>>5 | seed<<11
	return (key + 0xB988) & 0xFFFF
}

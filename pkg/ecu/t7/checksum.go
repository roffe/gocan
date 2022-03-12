package t7

type checksumArea struct {
	addr   uint32
	length uint16
}

func VerifyChecksum(bin []byte) error {
	var areas []checksumArea

	for i := 0; i < 16; i++ {
		areas = append(areas, checksumArea{})
	}

	return nil
}

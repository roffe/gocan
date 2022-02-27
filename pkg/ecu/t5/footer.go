package t5

import (
	"context"
	"fmt"
	"log"
	"strings"
)

var ecuFooter []byte

func (t *Client) GetECUFooter(ctx context.Context) ([]byte, error) {
	//if !t.bootloaded {
	//	if err := t.UploadBootLoader(ctx); err != nil {
	//		return err
	//	}
	//}
	if len(ecuFooter) > 0 {
		return ecuFooter, nil
	}

	footer := make([]byte, 0x80)
	var address uint32 = 0x7FF80 + 5

	for i := 0; i < (0x80 / 6); i++ {
		b, err := t.ReadMemoryByAddress(ctx, address)
		if err != nil {
			return nil, fmt.Errorf("failed to get ECU footer: %v", err)
		}
		for j := 0; j < 6; j++ {
			footer[(i*6)+j] = b[j]
		}
		address += 6
	}
	lastBytes, err := t.ReadMemoryByAddress(ctx, 0x7FFFF)
	if err != nil {
		return nil, fmt.Errorf("failed to get ECU footer: %v", err)
	}

	for j := 2; j < 6; j++ {
		footer[(0x80-6)+j] = lastBytes[j]
	}

	ecuFooter = footer
	return footer, nil
}

func GetIdentifierFromFooter(footer []byte, identifier byte) string {
	var result strings.Builder
	offset := len(footer) - 0x05 //  avoid the stored checksum
	for offset > 0 {
		length := int(footer[offset])
		offset--
		search := footer[offset]
		offset--
		if identifier == search {
			for i := 0; i < length; i++ {
				result.WriteByte(footer[offset])
				offset--
			}
			return result.String()
		}
		offset -= length
	}
	log.Printf("error getting identifier 0x%X", identifier)
	return ""
}

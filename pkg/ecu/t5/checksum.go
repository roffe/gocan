package t5

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"time"

	"github.com/roffe/gocan"
)

func (t *Client) GetECUChecksum(ctx context.Context) ([]byte, error) {
	if !t.bootloaded {
		if err := t.UploadBootLoader(ctx, nil); err != nil {
			return nil, err
		}
	}
	frameData := gocan.NewFrame(0x5, []byte{0xC8, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}, gocan.ResponseRequired)
	resp, err := t.c.SendAndPoll(ctx, frameData, 1*time.Second, 0xC)
	if err != nil {
		return nil, fmt.Errorf("failed to get ECU checksum: %v", err)
	}
	data := resp.Data()[2:6]
	return data, nil
}

func (t *Client) CalculateBinChecksum(bin []byte) ([]byte, error) {
	codeLen := getCodeLength(bin)
	if codeLen < 0 {
		return nil, errors.New("could not find end marker in bin")
	}
	var calculated uint32
	for pos := 0; int64(pos) <= codeLen; pos++ {
		calculated += uint32(bin[pos])
	}
	out := make([]byte, 4)
	binary.BigEndian.PutUint32(out, calculated)

	return out, nil
}

// Find the end marker in bin and report back the code length
func getCodeLength(bin []byte) int64 {
	ix := 0
	search := []byte{0x4E, 0xFA, 0xFB, 0xCC}
	r := bufio.NewReader(bytes.NewReader(bin))
	offset := int64(0)
	for ix < len(search) {
		b, err := r.ReadByte()
		if err != nil {
			return -1
		}
		if search[ix] == b {
			ix++
		} else {
			ix = 0
		}
		offset++
	}
	return offset - int64(len(search)) + 3
}

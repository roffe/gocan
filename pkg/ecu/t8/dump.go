package t8

import (
	"bytes"
	"context"
	"crypto/md5"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/avast/retry-go"
	"github.com/roffe/gocan"
	"github.com/roffe/gocan/pkg/model"
)

func (t *Client) DumpECU(ctx context.Context, callback model.ProgressCallback) ([]byte, error) {
	if err := t.Bootstrap(ctx, callback); err != nil {
		return nil, err
	}

	if callback != nil {
		callback("Dumping ECU")
	}
	start := time.Now()

	bin, err := t.ReadFlashLegion(ctx, EcuByte_T8, 0x100000, false, callback)
	if err != nil {
		return nil, err
	}

	if callback != nil {
		callback("Verifying MD5..")
	}

	ecumd5bytes, err := t.LegionIDemand(ctx, 0x02, 0x00)
	if err != nil {
		return nil, err
	}
	calculatedMD5 := md5.Sum(bin)

	if callback != nil {
		callback(fmt.Sprintf("Legion MD5 : %X", ecumd5bytes))
		callback(fmt.Sprintf("Local MD5  : %X", calculatedMD5))
	}

	if !bytes.Equal(ecumd5bytes, calculatedMD5[:]) {
		return nil, errors.New("MD5 Verification failed")
	}

	if callback != nil {
		callback("Done, took: " + time.Since(start).String())
		callback("Exiting bootloader")
	}
	if err := t.LegionExit(ctx); err != nil {
		return nil, err
	}

	return bin, nil
}

func (t *Client) ReadFlashLegion(ctx context.Context, device byte, lastAddress int, z22se bool, callback model.ProgressCallback) ([]byte, error) {
	if !t.legionRunning {
		if err := t.Bootstrap(ctx, callback); err != nil {
			return nil, err
		}
	}

	buf := make([]byte, lastAddress)
	bufpnt := 0

	// Pre-fill buffer with 0xFF (unprogrammed FLASH chip value)
	buf[0] = 0xFF
	for j := 1; j < len(buf); j *= 2 {
		copy(buf[j:], buf[:j])
	}

	if callback != nil {
		callback(-float64(lastAddress))
		callback(float64(0))
		callback("Downloading " + strconv.Itoa(lastAddress) + " bytes")
	}

	startAddress := 0
	var blockSize byte = 0x80

	for startAddress < lastAddress && ctx.Err() == nil {
		if callback != nil {
			callback(float64(startAddress + int(blockSize)))
		}
		err := retry.Do(
			func() error {
				b, blocksToSkip, err := t.readDataByLocalIdentifier(ctx, callback, true, EcuByte_T8, startAddress, blockSize)
				if err != nil {
					return err
				}
				if blocksToSkip > 0 {
					//callback("Skipping " + strconv.Itoa(blocksToSkip) + " blocks")
					bufpnt += (blocksToSkip * int(blockSize))
					startAddress += (blocksToSkip * int(blockSize))
				} else if len(b) == int(blockSize) {
					for j := 0; j < int(blockSize); j++ {
						buf[bufpnt] = b[j]
						bufpnt++
					}
					startAddress += int(blockSize)
				} else {
					return fmt.Errorf("dropped frame, len: %d bs: %d", len(b), blockSize)
				}
				return nil
			},
			retry.Attempts(3),
			retry.Context(ctx),
			retry.LastErrorOnly(true),
			retry.OnRetry(func(n uint, err error) {
				if callback != nil {
					callback(fmt.Sprintf("Retry #%d %v", n, err))
				}
			}),
		)
		if err != nil {
			return nil, err
		}
	}
	if callback != nil {
		callback(float64(lastAddress))
	}
	return buf, nil
}

//func (t *Client) readDataByLocalIdentifier(ctx context.Context, legionMode bool, pci byte, address int, length byte) ([]byte, int, error) {
//	return t.sendReadDataByLocalIdentifier(ctx, legionMode, pci, address, length)
//}

func (t *Client) readDataByLocalIdentifier(ctx context.Context, callback model.ProgressCallback, legionMode bool, pci byte, address int, length byte) ([]byte, int, error) {
	retData := make([]byte, length)
	payload := []byte{pci, 0x21, length,
		byte(address >> 24),
		byte(address >> 16),
		byte(address >> 8),
		byte(address),
		0x00,
	}

	frame := gocan.NewFrame(0x7E0, payload, gocan.ResponseRequired)

	resp, err := t.c.SendAndPoll(ctx, frame, t.defaultTimeout, 0x7E8)
	if err != nil {
		return nil, 0, err
	}

	if err := checkErr(resp); err != nil {
		return nil, 0, err
	}

	rx_cnt := 0
	var seq byte = 0x21
	d := resp.Data()

	if length <= 4 {
		for i := 4; i < int(4+length); i++ {
			if int(length) > rx_cnt {
				retData[rx_cnt] = d[i]
				rx_cnt++
			}
		}
		//log.Fatal("jo")
		return retData, 0, nil
	}

	for i := 0; i < 4; i++ {
		retData[i] = d[i+4]
		rx_cnt++
	}

	if !legionMode || d[3] == 0x00 {
		if err := t.c.SendFrame(0x7E0, []byte{0x30}, gocan.CANFrameType{Type: 2, Responses: 18}); err != nil {
			return nil, 0, err
		}
		m_nrFrameToReceive := ((length - 4) / 7)
		if (length-4)%7 > 0 {
			m_nrFrameToReceive++
		}

		c := t.c.Subscribe(ctx, 0x7E8)

		for m_nrFrameToReceive > 0 {
			select {
			case <-ctx.Done():
				return nil, 0, ctx.Err()
			case resp := <-c:
				d2 := resp.Data()

				if d2[0] != seq {
					errText := fmt.Sprintf("received invalid sequenced frame 0x%02X, expected 0x%02X", d2[0], seq)
					if callback != nil {
						callback(errText)
					}
					return nil, 0, errors.New(errText)
				}
				for i := 1; i < 8; i++ {
					if rx_cnt < int(length) {
						retData[rx_cnt] = d2[i]
						rx_cnt++
					}
				}
				seq++
				m_nrFrameToReceive--
				if seq > 0x2F {
					seq = 0x20
				}

			case <-time.After(1 * time.Second):
				return nil, 0, errors.New("timeout waiting for blocks")
			}
		}
	} else {
		// Loader tagged package as filled with FF
		// (Ie it's not necessary to send a go and receive the rest of the frame,
		// we already know what it contains)
		retData[0] = 0xFF
		for j := 1; j < len(retData); j *= 2 {
			copy(retData[j:], retData[:j])
		}
		return retData, int(d[3]), nil
	}

	return retData, 0, nil
}

//func (t *Client) getData(ctx context.Context, length, seq byte, callback model.ProgressCallback) ([]byte, int, error) {
//}

func checkErr(f gocan.CANFrame) error {
	d := f.Data()
	switch {
	case d[0] == 0x7E:
		return errors.New("got 0x7E message as response to 0x21, ReadDataByLocalIdentifier command")
	case bytes.Equal(d, []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}):
		return errors.New("got blank response message to 0x21, ReadDataByLocalIdentifier")
	case d[0] == 0x03 && d[1] == 0x7F && d[2] == 0x23:
		return errors.New("no security access granted")
	case d[2] != 0x61 && d[1] != 0x61:
		if bytes.Equal(d, []byte{0x01, 0x7E, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}) {
			return fmt.Errorf("incorrect response to 0x21, sendReadDataByLocalIdentifier.  Byte 2 was %X", d[2])
		}
	}
	return nil
}

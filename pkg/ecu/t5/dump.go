package t5

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"time"

	"github.com/roffe/gocan/pkg/model"
)

func (t *Client) DumpECU(ctx context.Context, callback model.ProgressCallback) ([]byte, error) {
	if !t.bootloaded {
		if err := t.UploadBootLoader(ctx, callback); err != nil {
			return nil, err
		}
	}

	ecutype, err := t.DetermineECU(ctx)
	if err != nil {
		return nil, err
	}

	start := getstartAddress(ecutype)
	length := 0x80000 - start

	if callback != nil {
		callback(-float64(length))
		callback("Dumping ECU")
	}

	buffer := make([]byte, length)
	startTime := time.Now()
	progress := 0

	address := start + 5
	for i := 0; i < int(length/6); i++ {
		b, err := t.ReadMemoryByAddress(ctx, address)
		if err != nil {
			return nil, err
		}
		for j := 0; j < 6; j++ {
			buffer[(i*6)+j] = b[j]
			progress++
		}
		address += 6
		if callback != nil {
			callback(float64(progress))
		}
	}

	// Get the leftover bytes
	if (length % 6) > 0 {
		b, err := t.ReadMemoryByAddress(ctx, start+length-1)
		if err != nil {
			return nil, err
		}
		for j := (6 - (length % 6)); j < 6; j++ {
			buffer[length-6+j] = b[j]
			progress++
		}
		if callback != nil {
			callback(float64(progress))
		}
	}

	if callback != nil {
		callback(float64(length))
		callback(fmt.Sprintf("Done, took: %s", time.Since(startTime).Round(time.Millisecond).String()))
	}

	//fmt.Printf("took: %s\n", time.Since(startTime).Round(time.Millisecond).String())

	checksum, err := t.GetECUChecksum(ctx)
	if err != nil {
		return nil, err
	}

	calculated, err := t.CalculateBinChecksum(buffer)
	if err != nil {
		return nil, err
	}

	if !bytes.Equal(checksum, calculated) {
		log.Println("!!! Dumped bin and calculated checksum from ECU does not match !!!")
		log.Printf("ECU reported checksum: %X, calculated: %X", checksum, calculated)
	}

	return buffer, nil
}

func getstartAddress(ecutype ECUType) uint32 {
	switch ecutype {
	case T52ECU, T55AST52:
		return 0x60000
	default:
		return 0x40000
	}
}

package t8

import (
	"bytes"
	"context"
	"crypto/md5"
	"errors"
	"fmt"
	"time"

	"github.com/roffe/gocan/pkg/legion"
	"github.com/roffe/gocan/pkg/model"
)

func (t *Client) DumpECU(ctx context.Context, callback model.ProgressCallback) ([]byte, error) {
	if err := t.legion.Bootstrap(ctx, callback); err != nil {
		return nil, err
	}

	if callback != nil {
		callback("Dumping ECU")
	}
	start := time.Now()

	bin, err := t.legion.ReadFlash(ctx, legion.EcuByte_T8, 0x100000, false, callback)
	if err != nil {
		return nil, err
	}

	if callback != nil {
		callback("Verifying md5..")
	}

	ecuMD5bytes, err := t.legion.IDemand(ctx, legion.GetTrionic8MD5, 0x00)
	if err != nil {
		return nil, err
	}
	calculatedMD5 := md5.Sum(bin)

	if callback != nil {
		callback(fmt.Sprintf("Legion MD5 : %X", ecuMD5bytes))
		callback(fmt.Sprintf("Local MD5  : %X", calculatedMD5))
	}

	if !bytes.Equal(ecuMD5bytes, calculatedMD5[:]) {
		return nil, errors.New("md5 Verification failed")
	}

	if callback != nil {
		callback("Done, took: " + time.Since(start).String())
	}

	return bin, nil
}

package t5

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/k0kubun/go-ansi"
	"github.com/schollz/progressbar/v3"
)

func (t *Client) DumpECU(ctx context.Context) ([]byte, error) {
	if !t.bootloaded {
		if err := t.UploadBootLoader(ctx); err != nil {
			return nil, err
		}
	}

	log.Println("Start ECU dump")
	ecutype, err := t.DetermineECU(ctx)
	if err != nil {
		return nil, err
	}

	var start uint32 = 0x40000

	if ecutype == T52ECU || ecutype == T55AST52 {
		start = 0x60000
	}

	length := 0x80000 - start
	buffer := make([]byte, length)

	//	buff := bytes.NewBuffer(nil)

	log.Println("Downloading flash from ECU")
	startTime := time.Now()
	bar := progressbar.NewOptions(int(length),
		progressbar.OptionSetWriter(ansi.NewAnsiStdout()),
		progressbar.OptionEnableColorCodes(true),
		progressbar.OptionShowBytes(true),
		progressbar.OptionSetWidth(20),
		progressbar.OptionSetDescription("[cyan][1/1][reset] dumping ECU"),
		progressbar.OptionSetTheme(progressbar.Theme{
			Saucer:        "[green]=[reset]",
			SaucerHead:    "[green]>[reset]",
			SaucerPadding: " ",
			BarStart:      "[",
			BarEnd:        "]",
		}))
	defer func() {
		bar.Finish()
		fmt.Println()
		log.Printf("ecu dump took: %s", time.Since(startTime).Round(time.Millisecond).String())
	}()
	address := start + 5
	for i := 0; i < int(length/6); i++ {
		b, err := t.ReadMemoryByAddress(ctx, address)
		if err != nil {
			return nil, err
		}
		for j := 0; j < 6; j++ {
			buffer[(i*6)+j] = b[j]
			bar.Add(1)
		}
		address += 6
	}

	if (length % 6) > 0 {
		b, err := t.ReadMemoryByAddress(ctx, start+length-1)
		if err != nil {
			return nil, err
		}
		for j := (6 - (length % 6)); j < 6; j++ {
			buffer[length-6+j] = b[j]
			bar.Add(1)
		}
	}
	return buffer, nil
}

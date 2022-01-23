package cmd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"time"

	"github.com/roffe/canusb"
	"github.com/roffe/canusb/pkg/t7"
	"github.com/spf13/cobra"
)

var canCIMTOY = &cobra.Command{
	Use:   "cimtoy",
	Short: "cim toy",
	//Long:  `Flash binary to CIM`,
	Hidden: true,
	Args:   cobra.RangeArgs(0, 5),
	RunE: func(cmd *cobra.Command, args []string) error {
		log.SetFlags(log.Lshortfile | log.LstdFlags)
		ctx, cancel := context.WithCancel(cmd.Context())
		defer cancel()
		c, err := canusb.New(
			ctx,
			// CAN identifiers for filtering
			filters,
			// Set com-port options
			canusb.OptComPort(comPort, baudRate),
			// Set CAN bit-rate
			canusb.OptRate(t7.PBusRate),
		)
		if err != nil {
			return err
		}

		defer c.Close()

		go func() {
			for {
				c.SendFrame(0x101, []byte{0x01, 0x3e})
				time.Sleep(500 * time.Millisecond)
			}
		}()

		log.Println("Physically Requested TesterPresent (response required)")
		c.SendFrame(0x245, []byte{0x01, 0x3e}) // Physically Requested TesterPresent (response required)
		_, err = c.Poll(ctx, 150*time.Millisecond, 0x645)
		if err != nil {
			return err
		}

		log.Println("DisableNormalCommunication Request Message")
		c.SendFrame(0x101, []byte{0xFE, 0x01, 0x28}) // DisableNormalCommunication Request Message
		_, err = c.Poll(ctx, 150*time.Millisecond, 0x645)
		if err != nil {
			return err
		}

		var secLvl byte = 0x0b
		log.Println("SecurityAccess(requestSeed)")
		c.SendFrame(0x245, []byte{0x02, 0x27, secLvl})
		f, err := c.Poll(ctx, 100*time.Millisecond, 0x645)
		if err != nil {
			log.Println(err)
			return err
		}
		time.Sleep(3 * time.Millisecond)

		auth := convertSeedCIM(int(f.Data[3])<<8 | int(f.Data[4]))

		log.Println("SecurityAccess (sendKey)")
		c.SendFrame(0x245, []byte{0x04, 0x27, secLvl + 1, byte(int(auth) >> 8 & int(0xFF)), byte(auth) & 0xFF})
		f, err = c.Poll(ctx, 100*time.Millisecond, 0x645)
		if err != nil {
			log.Println(err)
			return os.ErrDeadlineExceeded
		}
		if f.Data[1] != 0x67 && f.Data[2] == 0x02 {
			log.Println("sec access failed")
			return err
		}

		readDiagnosticInformation(ctx, c)
		reportProgrammingState(ctx, c)

		b, err := readDataByIdentifier(ctx, c, 0x90)
		if err != nil {
			return err
		}
		log.Printf("VIN>> %X, %s", b, string(b[:]))

		time.Sleep(10 * time.Millisecond)

		log.Println("requestProgrammingMode")
		requestProgrammingMode := []byte{0xFE, 0x02, 0xA5, 0x1}
		c.SendFrame(0x101, requestProgrammingMode)
		presp, err := c.Poll(ctx, 150*time.Millisecond, 0x645)
		if err != nil {
			return err
		}
		if presp.Data[0] != 0x01 || presp.Data[1] != 0xE5 {
			log.Println(presp.String())
			return fmt.Errorf("invalid response to request programming mode")
		}
		if err := sendKeepAlive(ctx, c); err != nil {
			return err
		}
		log.Println("enableProgrammingMode")
		enableProgrammingMode := []byte{0xFE, 0x02, 0xA5, 0x03}
		c.SendFrame(0x245, enableProgrammingMode)

		time.Sleep(100 * time.Millisecond)

		if err := writeDataByIdentifier(ctx, c, 0x90, []byte("YS3FB56F091023064")); err != nil {
			return err
		}
		f2, err := c.Poll(ctx, 150*time.Millisecond, 0x645)
		if err != nil {
			log.Println(err)
			return err
		}
		log.Println(f2.String())

		b2, err := readDataByIdentifier(ctx, c, 0x90)
		if err != nil {
			return err
		}
		log.Printf("VIN>> %X, %s", b2, string(b2[:]))

		log.Println("ReturnToNormalMode Message Flow")
		c.SendFrame(0x245, []byte{0x01, 0x20}) //  ReturnToNormalMode Message Flow
		return nil
	},
}

func init() {
	canCMD.AddCommand(canCIMTOY)
}

func readDiagnosticInformation(ctx context.Context, c *canusb.Canusb) {
	log.Println("readDiagnosticInformation")
	c.SendFrame(0x245, []byte{0x05, 0xA9, 0x80, 0x07, 0x00, 0x02})
	f, err := c.Poll(ctx, 150*time.Millisecond)
	if err != nil {
		log.Println(err)
		return
	}
	log.Println(f.String())
}

func reportProgrammingState(ctx context.Context, c *canusb.Canusb) {
	log.Println("reportProgrammingState")
	c.SendFrame(0x101, []byte{0xFE, 0x01, 0xA2, 0xAA, 0xAA, 0xAA, 0xAA, 0xAA})
	f, err := c.Poll(ctx, 150*time.Millisecond)
	if err != nil {
		log.Println(err)
		return
	}
	log.Println(f.String())
}

func writeDataByIdentifier(ctx context.Context, c *canusb.Canusb, identifier byte, data []byte) error {
	r := bytes.NewReader(data)

	firstPart := make([]byte, 4)
	if _, err := r.Read(firstPart); err != nil {
		return err
	}
	payload := []byte{0x10, byte(len(data) + 2), 0x3B, identifier}
	payload = append(payload, firstPart...)
	c.SendFrame(0x245, payload)
	log.Printf("%X\n", payload)
	resp, err := c.Poll(ctx, 100*time.Millisecond, 0x645)
	if err != nil {
		return err
	}
	if resp.Data[0] != 0x30 || resp.Data[1] != 0x00 {
		log.Println(resp.String())
		return errors.New("invalid response to initial writeDataByIdentifier")
	}

	delay := resp.Data[2]

	var seq byte = 0x21

	for r.Len() > 0 {
		pkg := []byte{seq}
	inner:
		for i := 1; i < 8; i++ {
			b, err := r.ReadByte()
			if err != nil {
				if err == io.EOF {
					log.Println("eof")
					break inner
				}
				return err
			}
			pkg = append(pkg, b)
		}
		c.SendFrame(0x245, pkg)
		log.Printf("%X\n", pkg)
		time.Sleep(time.Duration(delay) * time.Millisecond)
		seq++
		if seq == 0x30 {
			seq = 0x20
		}
	}

	return nil
}

func readDataByIdentifier(ctx context.Context, c *canusb.Canusb, identifier byte) ([]byte, error) {
	out := bytes.NewBuffer([]byte{})
	c.SendFrame(0x245, []byte{0x02, 0x1A, identifier})
	resp, err := c.Poll(ctx, 100*time.Millisecond, 0x645)
	if err != nil {
		return nil, err
	}
	if resp.Data[3] == 0x78 {
		resp, err = c.Poll(ctx, 150*time.Millisecond, 0x645)
		if err != nil {
			return nil, err
		}
		out.Write(resp.Data[4:])
	} else {
		out.Write(resp.Data[4:])
	}

	left := int(resp.Data[1])
	left -= 6
	c.SendFrame(0x245, []byte{0x30, 0x00, 0x00})

outer:
	for left > 0 {
		read, err := c.Poll(ctx, 100*time.Millisecond, 0x645)
		if err != nil {
			return nil, err
		}
		for _, b := range read.Data[1:] {
			out.WriteByte(b)
			left--
			if left == 0 {
				break outer
			}
		}

	}

	return out.Bytes(), nil
}

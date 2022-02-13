package cmd

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/avast/retry-go"
	"github.com/roffe/gocan"
	"github.com/roffe/gocan/pkg/model"
	"github.com/spf13/cobra"
)

var cimDump = &cobra.Command{
	Use:   "cimdump",
	Short: "toy stuff",
	//Long:  `Flash binary to ECU`,
	Hidden: true,
	Args:   cobra.RangeArgs(0, 5),
	RunE: func(cmd *cobra.Command, args []string) error {
		log.SetFlags(log.Lshortfile | log.LstdFlags)
		ctx, cancel := context.WithCancel(cmd.Context())
		defer cancel()

		adapter, port, baudrate, err := getAdapterOpts()
		if err != nil {
			return err
		}
		c, err := initCAN(ctx, adapter, port, baudrate)
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
		d := f.GetData()
		resp := convertSeedCIM(int(d[3])<<8 | int(d[4]))

		log.Println("SecurityAccess (sendKey)")
		c.SendFrame(0x245, []byte{0x04, 0x27, secLvl + 1, byte(int(resp) >> 8 & int(0xFF)), byte(resp) & 0xFF})
		f2, err := c.Poll(ctx, 100*time.Millisecond, 0x645)
		if err != nil {
			log.Println(err)
			return os.ErrDeadlineExceeded
		}
		d2 := f2.GetData()
		if d2[1] != 0x67 && d2[2] == 0x02 {
			log.Println("sec access failed")
			return err
		}

		ranges := []struct {
			Start  uint32
			Length uint32
			MaxLen uint32
		}{
			{
				Start:  0x100000,
				Length: 0x3FF,
				MaxLen: 25,
			},
			{
				Start:  0x100400,
				Length: 0x13FF,
				MaxLen: 25,
			},
			{
				Start:  0x200000,
				Length: 0x1a,
				MaxLen: 25,
			},
			{
				Start:  0x800000,
				Length: 0x110,
				MaxLen: 4,
			},
		}

		for _, r := range ranges {
			log.Printf("reading 0x%X - 0x%X", r.Start, r.Start+r.Length)
			d, err := readRange(ctx, c, r.Start, r.Length, r.MaxLen)
			if err != nil {
				return err
			}
			log.Printf("%X\n", d)
		}

		time.Sleep(300 * time.Millisecond)
		log.Println("ReturnToNormalMode Message Flow")
		c.SendFrame(0x245, []byte{0x01, 0x20}) //  ReturnToNormalMode Message Flow
		return nil
	},
}

func readRange(ctx context.Context, c *gocan.Client, start, length, maxlen uint32) ([]byte, error) {
	lastKeepAlive := time.Now()

	out := bytes.NewBuffer([]byte{})

	end := start + length
	curOffset := start
	var readBytes byte

	for curOffset < end {
		if end-curOffset >= maxlen {
			readBytes = byte(maxlen)
		} else {
			readBytes = byte(end - curOffset)
		}
		leftThisRound := int(readBytes)
		if time.Since(lastKeepAlive) >= 2*time.Second {
			//			log.Printf("ReadMemoryByAddress: 0x%02X len: %d\n", curOffset, readBytes)
			c.SendFrame(0x245, []byte{0x01, 0x3e}) // Physically Requested TesterPresent (response required)
			_, err := c.Poll(ctx, 150*time.Millisecond, 0x645)
			if err != nil {
				return nil, err
			}
			lastKeepAlive = time.Now()
		}

		bs := make([]byte, 4)
		binary.BigEndian.PutUint32(bs, curOffset)
		payload := []byte{0x06, 0x23, bs[1], bs[2], bs[3], 0x00, readBytes}
		log.Printf("%X %X\n", curOffset, bs)
		log.Printf("payload>>%X\n", payload)
		var fs model.CANFrame
		err := retry.Do(
			func() error {
				var err error
				c.SendFrame(0x245, payload) //ReadMemoryByAddress
				ff, err := c.Poll(ctx, 150*time.Millisecond, 0x645)
				if err != nil {
					return err
				}
				fs = ff
				// $7F $23 $78
				d := ff.GetData()
				if d[1] == 0x7f {
					if d[2] == 0x23 && d[3] == 0x78 { // plez retry
						ff2, err := c.Poll(ctx, 200*time.Millisecond, 0x645)
						if err != nil {
							return err
						}
						fs = ff2
						return nil
					}
					log.Println(ff.String())
					return fmt.Errorf("reading 0x%02X%02X%02X len: %d failed", bs[1], bs[2], bs[3], readBytes)
				}
				return nil
			},
			retry.Context(ctx),
			retry.Attempts(3),
		)
		if err != nil {
			return nil, err
		}

		if leftThisRound <= 3 {
			for i := 0; i <= 2; i++ {
				out.WriteByte(fs.GetData()[5+i])
				curOffset++
				leftThisRound--
				if leftThisRound == 0 {
					break
				}
			}
		}

		if leftThisRound > 3 {
			out.WriteByte(fs.GetData()[6])
			curOffset++
			leftThisRound--
			out.WriteByte(fs.GetData()[7])
			curOffset++
			leftThisRound--

			c.SendFrame(0x245, []byte{0x30, 0x00, 0x00}) // Send me moar!

		inner:
			for {
				f2, err := c.Poll(ctx, 150*time.Millisecond, 0x645)
				if err != nil {
					return nil, fmt.Errorf("failed to read additional data: %v", err)
				}
				for i := 1; i < 8; i++ {
					out.WriteByte(f2.GetData()[i])
					curOffset++
					leftThisRound--
					if leftThisRound == 0 {
						break inner
					}
				}
			}

		}
		time.Sleep(2 * time.Microsecond)
	}
	return out.Bytes(), nil
}

/*
func ignore(id uint32) bool {
	switch id {
	case 0x0C1, 0x0C5, 0x180, 0x1F5, 0x380, 0x381:
		return true
	}
	return false
}
*/
func init() {
	cimCmd.AddCommand(cimDump)
}

func convertSeedCIM(seed int) int {
	//log.Printf("converting seed: 0x%03X\n", seed)
	key := (seed + 0x9130) & 0xFFFF
	key = (key >> 8) | (key << 8)
	return (0x3FC7 - key) & 0xFFFF
}

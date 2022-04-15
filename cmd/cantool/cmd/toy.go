package cmd

import (
	"errors"
	"log"
	"time"

	"github.com/roffe/gocan"
	"github.com/roffe/gocan/pkg/sid"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(toyCmd)
}

var toyCmd = &cobra.Command{
	Use:    "toy",
	Short:  "toy",
	Hidden: true,
	Args:   cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		c, err := initCAN(ctx, 0x368)
		if err != nil {
			return err
		}
		defer c.Close()
		s := sid.New(c)

		cc := c.Subscribe(ctx)
		go func() {
			for {
				f := <-cc
				if wanted(f.Identifier()) {
					log.Println(f.String())
				}
			}
		}()

		log.SetFlags(log.LstdFlags | log.Lshortfile)

		//if err := s.StartRadioDisplay(ctx); err != nil {
		//	return err
		//}
		//s.Beep()

		//time.Sleep(30 * time.Millisecond)

		//if err := s.StartDiagnosticSession(ctx); err != nil {
		//	return err
		//}

		//var val byte = 0
		//for ctx.Err() == nil {
		//	log.Println(val)
		//	c.SendFrame(0x328, []byte{0x40, 0x96, 0x03, val, val, val, val, val}, gocan.Outgoing)
		//	time.Sleep(100 * time.Millisecond)
		//	val++
		//}

		//status, err := bitStringToBytes("10000001")
		//if err != nil {
		//	log.Fatal(err)
		//}

		_, err = s.RequestAccess(ctx)
		if err != nil {
			log.Println(err)
		}

		btext := sid.Translate("Fucking lights won't light   ")
		for ctx.Err() == nil {
			//c.SendFrame(0x3B0, []byte{0x80, 0x74, 0x7E, 0x20, 0x00, 0x00, 0x00, 0x00}, gocan.Outgoing)

			c.SendFrame(0x4A0, []byte{0x80, 0x37, 0x00, 0x20, 0x14, 0x95, 0x00}, gocan.Outgoing)
			s.SetRadioText(btext)
			btext = rotate(btext, 1)
			time.Sleep(200 * time.Millisecond)

		}

		/*
			s.SetRDSStatus(true, false, true, false, false, false)
			time.Sleep(600 * time.Millisecond)
			s.SetRDSStatus(false, false, false, false, false, false)
			time.Sleep(500 * time.Millisecond)

			s.SetRDSStatus(true, false, true, false, false, false)
			time.Sleep(400 * time.Millisecond)
			s.SetRDSStatus(false, false, false, false, false, false)
			time.Sleep(500 * time.Millisecond)

			_, err = s.RequestAccess(ctx)
			if err != nil {
				log.Println(err)
			}
			time.Sleep(30 * time.Millisecond)
			s.SetRadioText("there's no limit!")
			time.Sleep(800 * time.Millisecond)

			btext := sid.Translate("Saab power!                 ")
			for i := 0; i < 29; i++ {
				if ctx.Err() != nil {
					break
				}
				if err := s.SetRadioText(btext); err != nil {
					return err
				}
				btext = rotateString(btext, 1)
				time.Sleep(100 * time.Millisecond)
			}

			s.Beep()

			for i := 0; i < 25; i++ {
				msg := "B" + strings.Repeat("=", i) + "3"
				if err := s.SetRadioText(msg); err != nil {
					return err
				}
				time.Sleep(100 * time.Millisecond)
			}
			for i := 20; i > 0; i-- {
				mm := strings.Repeat(" ", i) + "<o))))><"
				if err := s.SetRadioText(mm); err != nil {
					return err
				}
				time.Sleep(100 * time.Millisecond)
			}
		*/

		return nil
	},
}

func wanted(id uint32) bool {
	switch id {
	case 0x6a6, 0x310, 0x368, 0x290, 0x410, 0x7a0, 0x730:
		return false
	default:
		return true
	}
}

func rotate(nums []byte, k int) []byte {
	k = k % len(nums)
	nums = append(nums[k:], nums[0:k]...)
	return nums
}

func rotateString(str string, k int) string {
	nums := []byte(str)
	k = k % len(nums)
	nums = append(nums[k:], nums[0:k]...)
	return string(nums)
}

var ErrRange = errors.New("value out of range")

func bitStringToBytes(s string) ([]byte, error) {
	b := make([]byte, (len(s)+(8-1))/8)
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < '0' || c > '1' {
			return nil, ErrRange
		}
		b[i>>3] |= (c - '0') << uint(7-i&7)
	}
	return b, nil
}

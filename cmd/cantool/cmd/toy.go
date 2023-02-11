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
		c, err := initCAN(ctx, 0)
		if err != nil {
			return err
		}
		defer c.Close()
		s := sid.New(c)

		cc := c.Subscribe(ctx)

		showMenu := false

		lastChange := time.Now()

		go func() {
			for {
				f := <-cc
				if wanted(f.Identifier()) {
					log.Println(f.String())
				}
				d := f.Data()
				if d[5] == 0xC0 {
					if time.Since(lastChange) > 400*time.Millisecond {
						s.Beep()
						if !showMenu {
							err = s.SIDAudioTextControl(ctx, 0, 1, 0x19)
							if err != nil {
								log.Println(err)
							}
							time.Sleep(10 * time.Millisecond)
							showMenu = true
						} else {
							showMenu = false
						}
						lastChange = time.Now()
					}
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
		/*
			for ctx.Err() == nil {

				if showMenu {
					c.SendFrame(0x328, []byte{0x49, 0x96, 0x1, '1', 0x08, ' ', '2', 0x02}, gocan.Outgoing)
					time.Sleep(10 * time.Millisecond)
					c.SendFrame(0x328, []byte{0x08, 0x96, 0x1, '3', 0x03, ' ', '4', 0x04}, gocan.Outgoing)
					time.Sleep(10 * time.Millisecond)
					c.SendFrame(0x328, []byte{0x07, 0x96, 0x1, '5', 0x05, ' ', '6', 0x06}, gocan.Outgoing)
					time.Sleep(10 * time.Millisecond)
					c.SendFrame(0x328, []byte{0x06, 0x96, 0x1, '7', 0x07, ' ', '8', 0x08}, gocan.Outgoing)
					time.Sleep(10 * time.Millisecond)
					c.SendFrame(0x328, []byte{0x05, 0x96, 0x1, '9', 0x09, ' ', 'a', 0x0a}, gocan.Outgoing)
					time.Sleep(10 * time.Millisecond)
					c.SendFrame(0x328, []byte{0x04, 0x96, 0x2, 'b', 0x0b, ' ', 'c', 0x0c}, gocan.Outgoing)
					time.Sleep(10 * time.Millisecond)
					c.SendFrame(0x328, []byte{0x03, 0x96, 0x2, 'd', 0x0d, ' ', 'e', 0x0e}, gocan.Outgoing)
					time.Sleep(10 * time.Millisecond)
					c.SendFrame(0x328, []byte{0x02, 0x96, 0x2, 'f', 0x0f, ' ', 'g', 0x10}, gocan.Outgoing)
					time.Sleep(10 * time.Millisecond)
					c.SendFrame(0x328, []byte{0x01, 0x96, 0x2, 'h', 0x11, ' ', 'i', 0x12}, gocan.Outgoing)
					time.Sleep(10 * time.Millisecond)
					c.SendFrame(0x328, []byte{0x00, 0x96, 0x2, 'j', 0x13, ' ', 'k', 0x14}, gocan.Outgoing)
				}

				time.Sleep(400 * time.Millisecond)
			}
		*/

		go func() {
			for {
				err = s.SIDAudioTextControl(ctx, 0, 0x01, 0x19)
				if err != nil {
					log.Println(err)
				}
				time.Sleep(1000 * time.Millisecond)
			}
		}()

		//var char byte = 0

		//go func() {
		//	reader := bufio.NewReader(os.Stdin)
		//	for {
		//		reader.ReadString('\n')
		//		char++
		//
		//	}
		//}()

		for ctx.Err() == nil {
			/*
				c.SendFrame(0x328, []byte{0x44, 0x96, 0x01, 'M', 'a', 'r', 'k', ' '}, gocan.Outgoing)
				time.Sleep(10 * time.Millisecond)
				c.SendFrame(0x328, []byte{0x03, 0x96, 0x01, 'h', 'a', 's', ' ', 'a'}, gocan.Outgoing)
				time.Sleep(10 * time.Millisecond)
				c.SendFrame(0x328, []byte{0x02, 0x96, 0x01, ' ', 'h', 'u', 'g', 'e'}, gocan.Outgoing)
				time.Sleep(10 * time.Millisecond)
				c.SendFrame(0x328, []byte{0x01, 0x96, 0x01, ' ', 'D', 'O', 'N', 'G'}, gocan.Outgoing)
				time.Sleep(10 * time.Millisecond)
				c.SendFrame(0x328, []byte{0x00, 0x96, 0x01, '!', ' ', ' ', 0x00, 0x00}, gocan.Outgoing)
				time.Sleep(30 * time.Millisecond)
			*/

			c.SendFrame(0x328, []byte{0x49, 0x96, 0x01, 'M', 'a', 'r', 'k', ' '}, gocan.Outgoing)
			time.Sleep(7 * time.Millisecond)
			c.SendFrame(0x328, []byte{0x08, 0x96, 0x01, 'h', 'a', 's', ' ', 'a'}, gocan.Outgoing)
			time.Sleep(7 * time.Millisecond)
			c.SendFrame(0x328, []byte{0x07, 0x96, 0x01, ' ', 'h', 'u', 'g', 'e'}, gocan.Outgoing)
			time.Sleep(7 * time.Millisecond)
			c.SendFrame(0x328, []byte{0x06, 0x96, 0x01, ' ', 'D', 0xD8, 'N', 'G'}, gocan.Outgoing)
			time.Sleep(7 * time.Millisecond)
			c.SendFrame(0x328, []byte{0x05, 0x96, 0x01, '!', ' ', ' ', ' ', ' '}, gocan.Outgoing)
			time.Sleep(30 * time.Millisecond)

			c.SendFrame(0x328, []byte{0x04, 0x96, 0x02, '<', '3', ' ', '<', '3'}, gocan.Outgoing)
			time.Sleep(7 * time.Millisecond)
			c.SendFrame(0x328, []byte{0x03, 0x96, 0x02, ' ', '<', '3', ' ', '<'}, gocan.Outgoing)
			time.Sleep(7 * time.Millisecond)
			c.SendFrame(0x328, []byte{0x02, 0x96, 0x02, '3', ' ', ' ', ' ', ' '}, gocan.Outgoing)
			time.Sleep(7 * time.Millisecond)
			c.SendFrame(0x328, []byte{0x01, 0x96, 0x02, ' ', ' ', ' ', ' ', ' '}, gocan.Outgoing)
			time.Sleep(7 * time.Millisecond)
			c.SendFrame(0x328, []byte{0x00, 0x96, 0x02, ' ', ' ', ' ', '<', '3'}, gocan.Outgoing)

			//c.SendFrame(0x3B0, []byte{0x80, 0x74, 0x7E, 0x20, 0x00, 0x00, 0x00, 0x00}, gocan.Outgoing)
			//c.SendFrame(0x4A0, []byte{0x0F, 0x37, 0x00, 0x20, 0x14, 0x95, 0x00}, gocan.Outgoing)
			//btext := fmt.Sprintf("0x%02X: %c", char, char)
			//if err := s.SetRadioText([]byte("korven")); err != nil {
			//	return err
			//}
			//btext = rotate(btext, 1)
			//c.SendFrame(0x430, []byte{0x80, sounds[char], 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}, gocan.Outgoing)
			//reader := bufio.NewReader(os.Stdin)
			//reader.ReadString('\n')

			//char++
			time.Sleep(500 * time.Millisecond)

		}
		//time.Sleep(10 * time.Millisecond)

		/*

			var canID uint32 = 0x337

			_, err = s.RequestAccess(ctx, 0, 1, 0x12)
			if err != nil {
				log.Println(err)
			}
			for ctx.Err() == nil {
				c.SendFrame(canID, []byte{0x49, 0x96, 0x01, 'M', 'a', 'r', 'k', ' '}, gocan.Outgoing)
				time.Sleep(10 * time.Millisecond)
				c.SendFrame(canID, []byte{0x08, 0x96, 0x01, 'h', 'a', 's', ' ', 'a'}, gocan.Outgoing)
				time.Sleep(10 * time.Millisecond)
				c.SendFrame(canID, []byte{0x07, 0x96, 0x01, ' ', 'h', 'u', 'g', 'e'}, gocan.Outgoing)
				time.Sleep(10 * time.Millisecond)
				c.SendFrame(canID, []byte{0x06, 0x96, 0x01, ' ', 'D', 'O', 'N', 'G'}, gocan.Outgoing)
				time.Sleep(10 * time.Millisecond)
				c.SendFrame(canID, []byte{0x05, 0x96, 0x01, '!', ' ', ' ', ' ', ' '}, gocan.Outgoing)
				time.Sleep(30 * time.Millisecond)

				c.SendFrame(canID, []byte{0x04, 0x96, 0x02, '<', '3', ' ', '<', '3'}, gocan.Outgoing)
				time.Sleep(10 * time.Millisecond)
				c.SendFrame(canID, []byte{0x03, 0x96, 0x02, ' ', '<', '3', ' ', '<'}, gocan.Outgoing)
				time.Sleep(10 * time.Millisecond)
				c.SendFrame(canID, []byte{0x02, 0x96, 0x02, '3', ' ', ' ', ' ', ' '}, gocan.Outgoing)
				time.Sleep(10 * time.Millisecond)
				c.SendFrame(canID, []byte{0x01, 0x96, 0x02, ' ', ' ', ' ', ' ', ' '}, gocan.Outgoing)
				time.Sleep(10 * time.Millisecond)
				c.SendFrame(canID, []byte{0x00, 0x96, 0x02, ' ', ' ', ' ', '<', '3'}, gocan.Outgoing)

				time.Sleep(300 * time.Millisecond)
			}
		*/

		s.SetRDSStatus(true, true, true, true, true, true)
		/*
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

		<-ctx.Done()
		return nil
	},
}

func wanted(id uint32) bool {
	switch id {
	case 0x6a6, 0x310, 0x410, 0x7a0, 0x730, 0x290:
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

package cmd

import (
	"bytes"
	"encoding/binary"
	"log"
	"strings"
	"time"

	"github.com/roffe/gocan/cmd/cantool/pkg/ui/srec"
	"github.com/roffe/gocan/pkg/t5"
	"github.com/spf13/cobra"
)

// t5Cmd represents the t5 command
var t5toyCmd = &cobra.Command{
	Use:   "toy",
	Short: "Trionic 5 toy",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		c, err := initCAN(ctx, 0xC)
		if err != nil {
			return err
		}
		defer c.Close()

		r := strings.NewReader(t5.MyBooty)
		sr := srec.NewSrec()
		if err := sr.Parse(r); err != nil {
			return err
		}

		//in := c.Subscribe(ctx)
		//go func() {
		//	for {
		//		msg := <-in
		//		if msg == nil {
		//			return
		//		}
		//		log.Println(msg.String())
		//	}
		//}()

		/*
			//		log.Println(sr.String())
			for ctx.Err() == nil {
				log.Println("loop")
				c.SendFrame(0x5, neger('S'))
				time.Sleep(5 * time.Millisecond)
				c.SendFrame(0x5, neger(0x0D))
				time.Sleep(3000 * time.Millisecond)
			}
		*/
		c.SendFrame(0x005, []byte{0xA5, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00})
		f, err := c.Poll(ctx, 150*time.Millisecond)
		if err != nil {
			return err
		}
		log.Println(f.String())

		c.SendFrame(0x005, []byte{0xA5, 0x00, 0x00, 0x50, 0x00, 0x020, 0x00, 0x00})
		f, err = c.Poll(ctx, 150*time.Millisecond)
		if err != nil {
			return err
		}
		log.Println(f.String())

		<-ctx.Done()
		return nil
	},
}

func init() {
	t5Cmd.AddCommand(t5toyCmd)
}

func neger(in byte) []byte {
	var cmd uint64 = 0
	cmd |= uint64(in)
	cmd <<= 8
	cmd |= uint64(0xC4)
	cmd |= 0xFFFFFFFFFFFF0000

	buff := bytes.NewBuffer(nil)

	if err := binary.Write(buff, binary.BigEndian, cmd); err != nil {
		panic(err)
	}
	return buff.Bytes()
}

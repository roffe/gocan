package cmd

import (
	"log"
	"time"

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

		tr := t5.New(c)

		/*
			in := c.Subscribe(ctx)
			go func() {
				for {
					msg := <-in
					if msg == nil {
						return
					}
					log.Println(msg.String())
				}
			}()
		*/

		if err := tr.UploadBootLoader(ctx); err != nil {
			log.Println(err)
		}

		time.Sleep(5 * time.Millisecond)

		d, err := tr.DetermineECU(ctx)
		if err != nil {
			return err
		}

		log.Println(d)

		if err := tr.ResetECU(ctx); err != nil {
			return err
		}

		return nil
	},
}

func init() {
	t5Cmd.AddCommand(t5toyCmd)
}

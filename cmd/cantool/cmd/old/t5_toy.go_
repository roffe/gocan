package cmd

import (
	"log"

	"github.com/roffe/gocan/pkg/t5"
	"github.com/spf13/cobra"
)

// t5Cmd represents the t5 command
var t5toyCmd = &cobra.Command{
	Use:   "toy",
	Short: "Trionic 5 toy",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		c, err := initCAN(ctx)
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

		if err := tr.PrintECUInfo(ctx); err != nil {
			return err
		}

		sram, err := tr.GetSRAMSnapshot(ctx)
		if err != nil {
			return err
		}

		log.Printf("%X", sram)

		if err := tr.ResetECU(ctx); err != nil {
			return err
		}

		return nil
	},
}

func init() {
	t5Cmd.AddCommand(t5toyCmd)
}

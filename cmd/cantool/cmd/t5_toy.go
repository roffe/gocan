package cmd

import (
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

		if _, err := tr.DetermineECU(ctx); err != nil {
			return err
		}

		if err := tr.PrintECUInfo(ctx); err != nil {
			return err
		}

		/*
			bin, err := tr.DumpECU(ctx)
			if err != nil {
				return err
			}

			checksum, err := tr.GetChecksum(ctx)
			if err != nil {
				return err
			}
			log.Printf("FLASH Checksum: %X", checksum)

			if err := tr.VerifyBinChecksum(bin, checksum); err != nil {
				log.Printf("failed to verify downloaded bin checksum: %v", err)
			} else {
				log.Println("dump matches ECU checksum, this is a good read!")
			}

			if err := os.WriteFile("dump.bin", bin, 0644); err != nil {
				log.Printf("failed to write dump file: %v", err)
			}
		*/

		if err := tr.ResetECU(ctx); err != nil {
			return err
		}

		return nil
	},
}

func init() {
	t5Cmd.AddCommand(t5toyCmd)
}

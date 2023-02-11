package cmd

import (
	"log"

	"github.com/roffe/gocan/pkg/gmlan"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(uecCmd)
}

var uecCmd = &cobra.Command{
	Use:    "uec",
	Short:  "uec",
	Hidden: true,
	Args:   cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		log.SetFlags(log.LstdFlags | log.Lshortfile)
		ctx := cmd.Context()
		c, err := initCAN(ctx, 0)
		if err != nil {
			return err
		}
		defer c.Close()

		//leg := legion.New(c)
		gm := gmlan.New(c, 0x24F, 0x64F)
		/*
			gm.TesterPresentNoResponseAllowed()

			if err := gm.InitiateDiagnosticOperation(ctx, 0x02); err != nil {
				return err
			}

			if err := gm.DisableNormalCommunication(ctx); err != nil {
				return err
			}

			resp, err := gm.ReadDataByIdentifier(ctx, 0x40)
			if err != nil {
				return err
			}
			log.Printf("PWM Settings for Bulbs: %X", resp)

			resp, err := gm.ReadDataByIdentifierUint16(ctx, 0x4D)
			if err != nil {
				return err
			}
			log.Printf("Bulb Outage & Substitution: %d", resp)
		*/

		//		if err := gm.WriteDataByIdentifier(ctx, 0x4D, []byte{0x8B, 0xF1, 0x6B, 0x6B}); err != nil {
		//			return err
		//		}
		resp, err := gm.ReadDataByIdentifier(ctx, 0x4D)
		if err != nil {
			return err
		}
		log.Printf("DID 0x%02X: %X %s", 0x52, resp, resp)

		return nil
	},
}

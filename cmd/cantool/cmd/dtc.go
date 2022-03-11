package cmd

import (
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(dtcCmd)
}

var dtcCmd = &cobra.Command{
	Use:   "dtc <file>",
	Short: "show ecu DTC's",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		c, err := initCAN(ctx)
		if err != nil {
			return err
		}
		defer c.Close()

		//tr := t8.New(c)
		//gm := gmlan.New(c, 0x7e0, 0x7e8)
		/*
			tr, err := ecu.New(c, getECUType())
			if err != nil {
				return err
			}

			if err := tr.PrintECUInfo(ctx); err != nil {
				return err
			}

			bin, err := tr.DumpECU(ctx, infoCallback)
			if err != nil {
				return err
			}

			if err := os.WriteFile(args[0], bin, 0644); err != nil {
				log.Printf("failed to write dump file: %v", err)
			}

			if err := tr.ResetECU(ctx, infoCallback); err != nil {
				return err
			}
		*/
		return nil
	},
}

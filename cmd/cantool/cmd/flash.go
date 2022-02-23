package cmd

import (
	"os"

	"github.com/roffe/gocan/pkg/ecu"
	"github.com/spf13/cobra"
)

var flashCmd = &cobra.Command{
	Use:   "flash <filename>",
	Short: "flash binary to ECU",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		c, err := initCAN(ctx)
		if err != nil {
			return err
		}
		defer c.Close()

		tr, err := ecu.New(c, getECUType())
		if err != nil {
			return err
		}

		bin, err := os.ReadFile(args[0])
		if err != nil {
			return err
		}

		if err := tr.PrintECUInfo(ctx); err != nil {
			return err
		}

		if err := tr.EraseECU(ctx, nil); err != nil {
			return err
		}

		if err := tr.FlashECU(ctx, bin, nil); err != nil {
			return err
		}

		if err := tr.ResetECU(ctx, nil); err != nil {
			return err
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(flashCmd)
}

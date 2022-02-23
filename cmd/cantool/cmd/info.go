package cmd

import (
	"github.com/roffe/gocan/pkg/ecu"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(infoCmd)
}

var infoCmd = &cobra.Command{
	Use:   "info",
	Short: "print ECU info",
	Args:  cobra.NoArgs,
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

		if err := tr.PrintECUInfo(ctx); err != nil {
			return err
		}

		if err := tr.ResetECU(ctx, nil); err != nil {
			return err
		}

		return nil
	},
}

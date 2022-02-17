package cmd

import (
	"github.com/roffe/gocan/pkg/t5"
	"github.com/spf13/cobra"
)

// t5Cmd represents the t5 command
var t5infoCmd = &cobra.Command{
	Use:   "info",
	Short: "print ECU info",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		c, err := initCAN(ctx, 0xC)
		if err != nil {
			return err
		}
		defer c.Close()

		tr := t5.New(c)
		if err := tr.PrintECUInfo(ctx); err != nil {
			return err
		}
		if err := tr.ResetECU(ctx); err != nil {
			return err
		}
		return nil
	},
}

func init() {
	t5Cmd.AddCommand(t5infoCmd)
}

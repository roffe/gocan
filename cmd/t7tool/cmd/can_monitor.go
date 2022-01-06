package cmd

import (
	"github.com/spf13/cobra"
)

var monitorCMD = &cobra.Command{
	Use:   "monitor",
	Short: "Monitor the CANbus for packages",
	//Long:  `Flash binary to ECU`,
	Args: cobra.RangeArgs(0, 5),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()

		c, err := initCAN(ctx)
		if err != nil {
			return err
		}

		c.Monitor(ctx)
		return nil
	},
}

func init() {
	canCMD.AddCommand(monitorCMD)
}

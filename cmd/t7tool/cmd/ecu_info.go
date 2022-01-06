package cmd

import (
	"context"
	"time"

	"github.com/roffe/canusb/pkg/t7"
	"github.com/spf13/cobra"
)

// ecuCmd represents the ecu command
var infoCmd = &cobra.Command{
	Use:   "info",
	Short: "print ECU info",
	Long:  `Connect to the ECU over CAN and print the info from it`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := context.WithTimeout(cmd.Context(), 15*time.Second)
		defer cancel()

		c, err := initCAN(ctx, 0x238, 0x258)
		if err != nil {
			return err
		}

		tr := t7.New(c)

		if err := tr.Info(ctx); err != nil {
			return err
		}
		return nil
	},
}

func init() {
	ecuCmd.AddCommand(infoCmd)
}

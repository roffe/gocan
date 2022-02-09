package cmd

import (
	"context"
	"time"

	"github.com/roffe/gocan/pkg/t7"
	"github.com/spf13/cobra"
)

// ecuCmd represents the ecu command
var infoCmd = &cobra.Command{
	Use:   "info",
	Short: "print ECU info",
	Long:  `Connect to the ECU over CAN and print the info from it`,
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := context.WithTimeout(cmd.Context(), 15*time.Second)
		defer cancel()
		port, err := rootCmd.PersistentFlags().GetString(flagPort)
		if err != nil {
			return err
		}
		baudrate, err := rootCmd.PersistentFlags().GetInt(flagBaudrate)
		if err != nil {
			return err
		}
		c, err := initCAN(ctx, port, baudrate, 0x238, 0x258)
		if err != nil {
			return err
		}
		defer c.Close()

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

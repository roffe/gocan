package cmd

import (
	"context"

	"github.com/roffe/canusb"
	"github.com/roffe/canusb/pkg/t7"
	"github.com/spf13/cobra"
)

var canCMD = &cobra.Command{
	Use:   "can",
	Short: "CAN related commands",
	//Long:  `Flash binary to ECU`,
	Args: cobra.ExactArgs(1),
	//RunE: func(cmd *cobra.Command, args []string) error {
	//	return nil
	//},
}

func init() {
	rootCmd.AddCommand(canCMD)
}
func initCAN(ctx context.Context, filters ...uint32) (*canusb.Canusb, error) {
	c, err := canusb.New(
		ctx,
		// CAN identifiers for filtering
		filters,
		// Set com-port options
		canusb.OptComPort(comPort, baudRate),
		// Set CAN bit-rate
		canusb.OptRate(t7.PBusRate),
	)
	if err != nil {
		return nil, err
	}
	return c, nil
}

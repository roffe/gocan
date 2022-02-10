package cmd

import (
	"context"

	"github.com/roffe/gocan"
	"github.com/roffe/gocan/adapters/lawicel"
	"github.com/roffe/gocan/pkg/t7"
	"github.com/spf13/cobra"
)

var canCMD = &cobra.Command{
	Use:   "can",
	Short: "CAN related commands",
	Args:  cobra.ExactArgs(1),
}

func init() {
	rootCmd.AddCommand(canCMD)
}
func initCAN(ctx context.Context, port string, baudrate int, filters ...uint32) (*gocan.Client, error) {

	device := lawicel.NewCanusb()

	if err := device.SetPort(port); err != nil {
		return nil, err
	}
	if err := device.SetPortRate(baudrate); err != nil {
		return nil, err
	}
	if err := device.SetCANrate(t7.PBusRate); err != nil {
		return nil, err
	}
	if err := device.Init(); err != nil {
		return nil, err
	}

	c, err := gocan.New(
		ctx,
		device,  // Our CAN device
		filters, // CAN identifiers for filtering
	)
	if err != nil {
		return nil, err
	}
	return c, nil
}

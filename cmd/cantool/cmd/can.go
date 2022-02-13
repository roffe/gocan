package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/roffe/gocan"
	"github.com/roffe/gocan/adapter/lawicel"
	"github.com/roffe/gocan/adapter/obdlink"
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
func initCAN(ctx context.Context, adapter string, port string, baudrate int, filters ...uint32) (*gocan.Client, error) {

	var dev gocan.Adapter

	switch strings.ToLower(adapter) {
	case "canusb":
		dev = lawicel.NewCanusb()
	case "sx":
		dev = obdlink.NewSX()
	default:
		return nil, fmt.Errorf("unknown adapter %q", adapter)
	}

	if err := dev.SetPort(port); err != nil {
		return nil, err
	}
	if err := dev.SetPortRate(baudrate); err != nil {
		return nil, err
	}
	if err := dev.SetCANrate(t7.PBusRate); err != nil {
		return nil, err
	}
	if err := dev.Init(ctx); err != nil {
		return nil, err
	}

	c, err := gocan.New(
		ctx,
		dev,     // Our CAN device
		filters, // CAN identifiers for filtering
	)
	if err != nil {
		return nil, err
	}
	return c, nil
}

func getAdapterOpts() (adapter string, port string, baudrate int, err error) {
	port, err = rootCmd.PersistentFlags().GetString(flagPort)
	if err != nil {
		return
	}
	baudrate, err = rootCmd.PersistentFlags().GetInt(flagBaudrate)
	if err != nil {
		return
	}
	adapter, err = rootCmd.PersistentFlags().GetString(flagAdapter)
	if err != nil {
		return
	}
	return
}

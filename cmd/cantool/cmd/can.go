package cmd

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/roffe/gocan"
	"github.com/roffe/gocan/adapter/lawicel"
	"github.com/roffe/gocan/adapter/obdlink"
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
func initCAN(ctx context.Context, filters ...uint32) (*gocan.Client, error) {
	adapter, port, baudrate, canrate, err := getAdapterOpts()
	if err != nil {
		return nil, err
	}

	var dev gocan.Adapter
	switch strings.ToLower(adapter) {
	case "canusb":
		dev = lawicel.NewCanusb()
	case "sx", "obdlinksx":
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
	if err := dev.SetCANrate(canrate); err != nil {
		return nil, err
	}

	if err := dev.Init(ctx); err != nil {
		return nil, err
	}

	return gocan.New(ctx, dev, filters)
}

func getAdapterOpts() (adapter string, port string, baudrate int, canrate float64, err error) {
	pf := rootCmd.PersistentFlags()
	port, err = pf.GetString(flagPort)
	if err != nil {
		return
	}
	baudrate, err = pf.GetInt(flagBaudrate)
	if err != nil {
		return
	}
	adapter, err = pf.GetString(flagAdapter)
	if err != nil {
		return
	}
	crate, err := pf.GetString(flagCANRate)
	switch strings.ToLower(crate) {
	case "pbus":
		canrate = 500
	case "ibus":
		canrate = 47.619
	case "t5":
		canrate = 615.384
	default:
		f, errs := strconv.ParseFloat(crate, 64)
		if errs != nil {
			err = fmt.Errorf("invalid CAN rate: %q: %v", crate, err)
			return
		}
		canrate = f
	}
	return
}

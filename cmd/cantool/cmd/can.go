package cmd

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"

	"github.com/roffe/gocan"
	"github.com/roffe/gocan/adapter"
	"github.com/roffe/gocanflasher/pkg/ecu"
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
	adapterName, port, baudrate, canrate, err := getAdapterOpts()
	if err != nil {
		return nil, err
	}
	if len(filters) == 0 {
		filters = ecu.CANFilters(getECUType())
	}

	dev, err := adapter.New(
		adapterName,
		&gocan.AdapterConfig{
			Port:         port,
			PortBaudrate: baudrate,
			CANRate:      canrate,
			CANFilter:    filters,
		},
	)
	if err != nil {
		return nil, err
	}
	return gocan.New(ctx, dev)
}

func getECUType() ecu.Type {
	ecutemp, err := rootCmd.PersistentFlags().GetString(flagECUType)
	if err != nil {
		log.Fatal(err)
	}
	return ecu.FromString(ecutemp)
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
	canRate, err := pf.GetString(flagCANRate)
	if err != nil {
		return
	}

	switch strings.ToLower(canRate) {
	case "pbus":
		canrate = 500
	case "ibus":
		canrate = 47.619
	case "gmlan":
		canrate = 33.3
	case "t5":
		canrate = 615.384
	default:
		f, errs := strconv.ParseFloat(canRate, 64)
		if errs != nil {
			err = fmt.Errorf("invalid CAN rate: %q: %v", canRate, err)
			return
		}
		canrate = f
	}
	return
}
